package ipmanager

import (
	"errors"
	"fmt"
	"net"

	"github.com/cybertec-postgresql/yaim/config"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type IPManagerLocal struct {
	conf      *config.Config
	code      int
	result    string
	addresses []string
	label     string
}

func NewIPManagerLocal(conf *config.Config) (IPManagerLocal, error) {
	var i IPManagerLocal
	i.conf = conf
	i.label = conf.Iface + ":" + conf.Label
	if len(i.label) >= 16 {
		log.Fatal("The label to be used when registering ip addresses is longer than 16 characters: ", i.label)
	}
	return i, nil
}

func (i *IPManagerLocal) AddIP(ip string) error {
	iface, iface_err := netlink.LinkByName(i.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return iface_err
	}
	addr, addr_err := netlink.ParseAddr(ip + "/" + fmt.Sprint(i.conf.Mask) + " " + i.label)
	if addr_err != nil {
		log.Error("Unable to parse IP address: ", addr_err)
		return addr_err
	}
	err := netlink.AddrAdd(iface, addr)
	if err == nil {
		log.Info("Registered IP address: ", addr)
	}
	return err
}

func (i *IPManagerLocal) DeleteIP(ip string) error {
	iface, iface_err := netlink.LinkByName(i.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ", iface_err)
		return iface_err
	}
	addr, addr_err := netlink.ParseAddr(ip + "/" + fmt.Sprint(i.conf.Mask) + " " + i.label)
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

func (i *IPManagerLocal) CheckIP(ip string) error {
	iface, iface_err := netlink.LinkByName(i.conf.Iface)
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
		if addr.Label != i.label {
			continue
		}
		if addr.IPNet.String() == ip+"/"+fmt.Sprint(i.conf.Mask) {
			log.Debug("configured address matches queried address")
			return nil
		} else {
			log.Debug("configured address " + addr.IPNet.String() + " doesn't match queried address " + ip + "/" + fmt.Sprint(i.conf.Mask))
		}
	}
	return errors.New("IP address could not be found.")
}

func (i *IPManagerLocal) GetAllIP() ([]*net.IPNet, error) {
	var filteredAddrs []*net.IPNet
	iface, iface_err := netlink.LinkByName(i.conf.Iface)
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
		if addr.Label == i.label {
			filteredAddrs = append(filteredAddrs, addr.IPNet)
		}
	}
	return filteredAddrs, nil
}

func (i *IPManagerLocal) DeleteAllIP() {
	ips, err := i.GetAllIP()
	if err != nil {
		log.Error("Unable to get all registered addresses for deletion")
		log.Error(err)
		return
	}
	if len(ips) == 0 {
		//nothing to delete, apparently
		return
	}
	iface, iface_err := netlink.LinkByName(i.conf.Iface)
	if iface_err != nil {
		log.Error("Unable to obtain interface by name: ")
		log.Error(iface_err)
		return
	}
	for _, ip := range ips {
		addr, addr_err := netlink.ParseAddr(ip.IP.String() + "/" + fmt.Sprint(i.conf.Mask) + " " + i.label)
		if addr_err != nil {
			log.Error("Unable to parse IP address: ", ip.IP.String(), "/", fmt.Sprint(i.conf.Mask))
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
