package ipmanager

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cybertec-postgresql/yaim/config"
	"github.com/mdlayher/arp"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const (
	arpRequestOp = 1
	arpReplyOp   = 2
)

var (
	ethernetBroadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
)

type IPManagerLocal struct {
	conf      *config.Config
	code      int
	result    string
	addresses []string
	label     string
}

func NewIPManagerLocal(conf *config.Config) (IPManagerLocal, error) {
	var ipManLocal IPManagerLocal
	ipManLocal.conf = conf
	ipManLocal.label = conf.Iface + ":" + conf.Label
	if len(ipManLocal.label) >= 16 {
		log.Fatal("The label to be used when registering ip addresses is longer than 16 characters: ", ipManLocal.label)
	}
	return ipManLocal, nil
}

func (ipManLocal *IPManagerLocal) AddIP(ip string) error {
	iface, iface_err := netlink.LinkByName(ipManLocal.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return iface_err
	}
	addr, addr_err := netlink.ParseAddr(ip + "/" + fmt.Sprint(ipManLocal.conf.Mask) + " " + ipManLocal.label)
	if addr_err != nil {
		log.Error("Unable to parse IP address: ", addr_err)
		return addr_err
	}
	err := netlink.AddrAdd(iface, addr)
	if err == nil {
		log.Info("Registered IP address: ", addr)
		// We can only send gratuitous ARP requests for non-local interfaces.
		if ipManLocal.conf.Iface != "lo" {
			err := ipManLocal.arpSendGratuitous(iface, *addr)
			if err == nil {
				log.Info("Sent gratuitous arp request and reply after adding address")
			} else {
				// For now we'll do nothing besides logging the error.
				// If we're unable to send ARP requests on our own accord,
				// the OS might still do that for us when the ARP cache on neighbours runs out eventually.
				log.Error(err)
			}
		}
	}
	return err
}

func (ipManLocal *IPManagerLocal) DeleteIP(ip string) error {
	iface, iface_err := netlink.LinkByName(ipManLocal.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return iface_err
	}
	addr, addr_err := netlink.ParseAddr(ip + "/" + fmt.Sprint(ipManLocal.conf.Mask) + " " + ipManLocal.label)
	if addr_err != nil {
		log.Error("Unable to parse IP address: ", addr_err)
		return addr_err
	}
	err := netlink.AddrDel(iface, addr)
	if err == nil {
		log.Info("Deregistered IP address: ", addr)
	}
	return err
}

func (ipManLocal *IPManagerLocal) CheckIP(ip string) error {
	iface, iface_err := netlink.LinkByName(ipManLocal.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return iface_err
	}
	addrs, addrs_err := netlink.AddrList(iface, netlink.FAMILY_V4)
	if addrs_err != nil {
		log.Error("Unable to retrieve list of addresses: ", addrs_err)
		return addrs_err
	}
	for _, addr := range addrs {
		if addr.Label != ipManLocal.label {
			continue
		}
		if addr.IPNet.String() == ip+"/"+fmt.Sprint(ipManLocal.conf.Mask) {
			log.Debug("configured address matches queried address")
			return nil
		} else {
			log.Debug("configured address " + addr.IPNet.String() + " doesn't match queried address " + ip + "/" + fmt.Sprint(ipManLocal.conf.Mask))
		}
	}
	return errors.New("IP address could not be found.")
}

func (ipManLocal *IPManagerLocal) GetAllIP() ([]*net.IPNet, error) {
	var filteredAddrs []*net.IPNet
	iface, iface_err := netlink.LinkByName(ipManLocal.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return nil, iface_err
	}
	addrs, addrs_err := netlink.AddrList(iface, netlink.FAMILY_V4)
	if addrs_err != nil {
		log.Error("Unable to retrieve list of addresses: ", addrs_err)
		return nil, addrs_err
	}
	for _, addr := range addrs {
		if addr.Label == ipManLocal.label {
			filteredAddrs = append(filteredAddrs, addr.IPNet)
		}
	}
	return filteredAddrs, nil
}

func (ipManLocal *IPManagerLocal) DeleteAllIP() {
	ips, err := ipManLocal.GetAllIP()
	if err != nil {
		log.Error("Unable to get all registered addresses for deletion")
		log.Error(err)
		return
	}
	if len(ips) == 0 {
		//nothing to delete, apparently
		return
	}
	iface, iface_err := netlink.LinkByName(ipManLocal.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ")
		log.Error(iface_err)
		return
	}
	for _, ip := range ips {
		addr, addr_err := netlink.ParseAddr(ip.IP.String() + "/" + fmt.Sprint(ipManLocal.conf.Mask) + " " + ipManLocal.label)
		if addr_err != nil {
			log.Error("Unable to parse IP address: ", ip.IP.String(), "/", fmt.Sprint(ipManLocal.conf.Mask))
			log.Error(addr_err)
			continue
		}
		err := netlink.AddrDel(iface, addr)
		if err != nil {
			log.Error("Failed to delete IP address: ", addr)
			log.Error(err)
		} else {
			log.Info("Deregistered IP address: ", addr)
		}
	}
}

func (ipManLocal *IPManagerLocal) arpSendGratuitous(iface netlink.Link, addr netlink.Addr) error {
	for i := 0; i < ipManLocal.conf.RetryNum; i++ {
		//TODO: this is not too nice, the "interface" structs used by the netlink and net library are not compatible.
		interf, _ := net.InterfaceByIndex(iface.Attrs().Index)
		arpClient, err := arp.Dial(interf)
		if err != nil {
			log.Printf("Problems with producing the arp client: %s", err)
		} else {
			/* While RFC 2002 does not say whether a gratuitous ARP request or reply is preferred
			* to update ones neighbours' MAC tables, the Wireshark Wiki recommends sending both.
			*		https://wiki.wireshark.org/Gratuitous_ARP
			* This site also recommends sending a reply, as requests might be ignored by some hardware:
			*		https://support.citrix.com/article/CTX112701
			 */
			gratuitousReplyPackage, err := arp.NewPacket(
				arpReplyOp,
				iface.Attrs().HardwareAddr,
				addr.IP,
				iface.Attrs().HardwareAddr,
				addr.IP,
			)
			if err != nil {
				log.Printf("Gratuitous arp reply package is malformed: %s", err)
				return err
			}

			/* RFC 2002 specifies (in section 4.6) that a gratuitous ARP request
			* should "not set" the target Hardware Address (THA).
			* Since the arp package offers no option to leave the THA out, we specify the Zero-MAC.
			* If parsing that fails for some reason, we'll just use the local interface's address.
			* The field is probably ignored by the receivers' implementation anyway.
			 */
			arpRequestDestMac, err := net.ParseMAC("00:00:00:00:00:00")
			if err != nil {
				// not entirely RFC-2002 conform but better then nothing.
				arpRequestDestMac = iface.Attrs().HardwareAddr
			}

			gratuitousRequestPackage, err := arp.NewPacket(
				arpRequestOp,
				iface.Attrs().HardwareAddr,
				addr.IP,
				arpRequestDestMac,
				addr.IP,
			)
			if err != nil {
				log.Printf("Gratuitous arp request package is malformed: %s", err)
				return err
			}

			errReply := arpClient.WriteTo(gratuitousReplyPackage, ethernetBroadcast)
			if err != nil {
				log.Printf("Couldn't write to the arpClient: %s", errReply)
			} else {
				log.Println("Sent gratuitous ARP reply")
			}

			errRequest := arpClient.WriteTo(gratuitousRequestPackage, ethernetBroadcast)
			if err != nil {
				log.Printf("Couldn't write to the arpClient: %s", errRequest)
			} else {
				log.Println("Sent gratuitous ARP request")
			}

			if errReply != nil || errRequest != nil {
				// If something went wrong while sending the packages, we'll run through the main loop again
			} else {
				//TODO: think about whether to leave this out to achieve simple repeat sending of GARP packages
				//just leave the function, we must have sent both kinds of packages successfully
				return nil
			}
		}
		time.Sleep(time.Duration(ipManLocal.conf.RetryAfter) * time.Millisecond)
	}
	return errors.New("Failed to send gratuitous ARP.")
}
