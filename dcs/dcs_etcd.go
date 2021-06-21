package dcs

import (
	"context"
	"errors"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/coreos/etcd/client"
	"github.com/cybertec-postgresql/yaim/config"
)

type EtcdDcs struct {
	conf             *config.Config
	basepath         string
	cfg              client.Config
	cl               client.Client
	kapi             client.KeysAPI
	getRecursiveOpts *client.GetOptions
	getOpts          *client.GetOptions
	ttlSetOpts       *client.SetOptions
	dirSetOpts       *client.SetOptions
}

func NewEtcdDcs(conf *config.Config) (*EtcdDcs, error) {
	var err error
	var d EtcdDcs

	d.conf = conf
	d.basepath = conf.DcsNamespace + conf.DcsClusterName + "/"
	d.cfg = client.Config{
		Endpoints: d.conf.DcsEndpoints,
		Transport: client.DefaultTransport,
		//HeaderTimeoutPerRequest: time.Second,
		Username: d.conf.EtcdUser,
		Password: d.conf.EtcdPassword,
	}

	d.cl, err = client.New(d.cfg)
	if err != nil {
		log.Fatal("couldn't initialize etcd client", err)
	}
	d.kapi = client.NewKeysAPI(d.cl)

	d.getRecursiveOpts = &client.GetOptions{
		Recursive: true,
		Sort:      true,
		Quorum:    true,
	}
	d.getOpts = &client.GetOptions{
		Recursive: false,
		Sort:      true,
		Quorum:    true,
	}
	d.ttlSetOpts = &client.SetOptions{
		TTL: time.Duration(d.conf.TTL) * time.Millisecond,
	}
	d.dirSetOpts = &client.SetOptions{
		Dir:       true,
		PrevExist: client.PrevIgnore,
	}

	//create k/v structure if doesn't exist yet
	_, dirErr := d.kapi.Get(context.Background(), d.basepath+"nodes", d.getOpts)
	if dirErr != nil {
		_, dirErr = d.kapi.Set(context.Background(), d.basepath+"nodes", "", d.dirSetOpts)
		if dirErr != nil {
			log.Fatal("couldn't create nodes dir in etcd.", dirErr)
		}
	}
	_, dirErr = d.kapi.Get(context.Background(), d.basepath+"ips", d.getOpts)
	if dirErr != nil {
		_, dirErr = d.kapi.Set(context.Background(), d.basepath+"ips", "", d.dirSetOpts)
		if dirErr != nil {
			log.Fatal("couldn't create ips dir in etcd.", dirErr)
		}
	}
	return &d, nil
}

func (d *EtcdDcs) AdvertiseInDCS() {
	//create key for this node in the DCS, if it exists this will simply update the TTL.
	_, err := d.kapi.Set(context.Background(), d.basepath+"nodes/"+d.conf.Nodename, "healthy", d.ttlSetOpts)
	if err != nil {
		log.Print(err)
	}
}

// return true when the IP is present in DCS and marked with our own name or not marked at all
// for all other cases, we need to deregister the IP
func (d *EtcdDcs) CheckIpInDCS(ip string) bool {
	//create "marked" key for this node in the directory of ip in DCS
	resp, err := d.kapi.Get(context.Background(), d.basepath+"ips/"+ip, d.getRecursiveOpts)
	if err != nil {
		log.Error("Error in CheckIpInDCS() :")
		log.Error(err)
		return false
	}
	if len(resp.Node.Nodes) == 0 {
		//no keys in directory
		log.Print("Trying to retroactively mark locally registered IP address: " + ip + " in DCS")
		return d.MarkIpInDCS(ip)
	}
	for _, n := range resp.Node.Nodes {
		key := strings.TrimPrefix(n.Key, d.basepath+"ips/"+ip)
		if key == "marked" {
			if n.Value == d.conf.Nodename {
				log.Debug("Validated DCS marker for registered IP: ", ip)
				return true
			} else {
				log.Error("Found DCS marker by other yaim: "+n.Value+" for locally registered IP: ", ip)
				log.Error(err)
				return false
			}
		}
	}
	return true
}

func (d *EtcdDcs) MarkIpInDCS(ip string) (success bool) {
	opts := &client.SetOptions{
		PrevExist: client.PrevNoExist,
		TTL:       time.Duration(d.conf.TTL) * time.Millisecond,
	}

	//create "marked" key for this node in the directory of ip in DCS
	_, err := d.kapi.Set(context.Background(), d.basepath+"ips/"+ip+"/marked", d.conf.Nodename, opts)
	if err != nil {
		log.Print("Error in MarkIpInDCS() :", err)
		return false
	} else {
		log.Print("marked IP in etcd: ", ip)
	}
	return true
}

func (d *EtcdDcs) RefreshMarkIpInDCS(ip string) {
	opts := &client.SetOptions{
		PrevValue: d.conf.Nodename,
		TTL:       time.Duration(d.conf.TTL) * time.Millisecond,
		Refresh:   true,
	}

	//refresh "marked" key for this node in the directory of ip in DCS, only if the value (nodeName) is "ours".
	_, err := d.kapi.Set(context.Background(), d.basepath+"ips/"+ip+"/marked", "", opts)
	if err != nil {
		log.Error("Failed to update TTL for marked IP in etcd: ", ip)
		log.Error(err)
	} else {
		log.Print("Updated TTL for marked IP in etcd: ", ip)
	}
}

func (d *EtcdDcs) UnMarkIpInDCS(ip string) {
	opts := &client.DeleteOptions{
		PrevValue: d.conf.Nodename,
	}

	//remove "marked" key for this node in the directory of ip in DCS, only if the value (nodeName) is "ours".
	_, err := d.kapi.Delete(context.Background(), d.basepath+"ips/"+ip+"/marked", opts)
	if err != nil {
		log.Print("Error in UnMarkIpInDCS() :", err)
	}
	log.Print("removed mark for IP in etcd: ", ip)
}

func (d *EtcdDcs) UnMarkAllIPs(ips []string) {
	for _, ip := range ips {
		d.UnMarkIpInDCS(ip)
	}
}

func (d *EtcdDcs) GetNumberAdvertisments() (num int, err error) {
	//retrieve all advertised nodes
	resp, err := d.kapi.Get(context.Background(), d.basepath+"nodes", d.getRecursiveOpts)
	if err == nil {
		if resp.Node.Dir {
			return len(resp.Node.Nodes), nil
		} else {
			err = errors.New("No advertisments of any nodes (including my own, apparently) where found.")
		}
	}
	return -1, err
}

func (d *EtcdDcs) GetIPs() (IPs, ownMarkedIPs, unmarkedIPs []string, err error) {
	//retrieve all ipsc.
	resp, err := d.kapi.Get(context.Background(), d.basepath+"ips", d.getRecursiveOpts)
	if err != nil {
		return nil, nil, nil, err
	}
	if resp.Node.Dir == false {
		err = errors.New("The \"" + d.basepath + "ips\" path was no directory!")
		return nil, nil, nil, err
	}
	//var numIps []string
	for _, n := range resp.Node.Nodes {
		ip := strings.TrimPrefix(n.Key, d.basepath+"ips/")
		if n.Dir {
			//we only want to count the IP adresses that we can actually manage (by putting a key in the directory)
			IPs = append(IPs, ip)
			for _, nn := range n.Nodes {
				//If the first entry in the directory of this ip has a key of "marked", we'll count it as this IP being used by any yaim.
				if strings.TrimPrefix(nn.Key, d.basepath+"ips/"+ip+"/") == "marked" {
					log.Debug("marked value found!")
					//If the first entry in the directory of this ip has a value of our own nodeName, we'll count it as this IP being used by _this_ yaim.
					if nn.Value == d.conf.Nodename {
						log.Debug("our own marked value found!")
						ownMarkedIPs = append(ownMarkedIPs, strings.TrimPrefix(n.Key, d.basepath+"ips/"))
					}
				}
			}
			if len(n.Nodes) <= 0 {
				//IP not marked!
				unmarkedIPs = append(unmarkedIPs, strings.TrimPrefix(n.Key, d.basepath+"ips/"))
			}
		} else {
			log.Error("entries for IP addresses need to be directories, ", n.Key, " is a key.")
		}
	}
	return IPs, ownMarkedIPs, unmarkedIPs, err
}
