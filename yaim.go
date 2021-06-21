package main

// yaim: 	yet another ip manager
//
// written:	by Julian Markwort in 2018/2019 at Cybertec Schönig & Schönig GmbH.
// written:	also by Julian Markwort in 2021 at CYBERTEC PostgreSQL International GmbH.
// Mail:	julian.markwort@cybertec.at
// Mail: 	office@cybertec.at
// Website:	www.cybertec-postgresql.com

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/cybertec-postgresql/yaim/checker"
	"github.com/cybertec-postgresql/yaim/config"
	"github.com/cybertec-postgresql/yaim/dcs"
	"github.com/cybertec-postgresql/yaim/ipmanager"
)

// var configFile = flag.String("config", "./yaim.yml", "Location of the configuration file.")
// var versionHint = flag.Bool("version", false, "Show the version number.")

var (
	// yaim version definition
	version string = "0.0.1"
)

func main() {
	if (len(os.Args) > 1) && (os.Args[1] == "--version") {
		//			log.Print("version " + version)
		//			return nil, nil
		//		}
		fmt.Printf("version: %s\n", version)
		return
	}

	conf, err := config.NewConfig()
	if err != nil {
		fmt.Println("error while loading configuration")
		fmt.Println(err)
		return
	}

	checker, err := checker.NewChecker(conf)
	if err != nil {
		fmt.Println("error while initiating checker")
		fmt.Println(err)
		return
	}

	dcs, err := dcs.NewDcs(conf)
	if err != nil {
		fmt.Println("error while initiating DCS connector")
		fmt.Println(err)
		return
	}

	ipman, err := ipmanager.NewIPManagerLocal(conf)
	if err != nil {
		fmt.Println("error while initiating IP manager")
		fmt.Println(err)
		return
	}

	loop(conf, checker, dcs, ipman)
}

func loop(conf *config.Config, checker checker.Checker, dcs dcs.Dcs, ipman ipmanager.IPManagerLocal) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	for {
		// time.Sleep(time.Duration(conf.Interval) * time.Millisecond)
		log.Debug("loop!")

		var err error
		var healthy bool
		for i := 0; i < conf.RetryNum; i++ {
			healthy, err = checker.IsHealthy()
			if err != nil {
				log.Printf("encountered an error while determining health status.\n")
				log.Print(err)
			} else {
				break
			}
			time.Sleep(time.Duration(conf.RetryAfter) * time.Millisecond)
		}
		if err != nil {
			log.Print("too many retries")
		}

		if healthy == true {
			log.Print("Node is healthy.")
			cleanup(conf, dcs, ipman)
			register(dcs, ipman)
		} else {
			log.Print("Node is not healthy.")
			// TODO: make sure to drop addresses here.
			// we need to keep track of all registered virtual IP addresses, so we can drop them.
			// This will be useful to know when we stop the program as well.
		}
		select {
		// Example. Process to receive a message
		// case msg := <-receiveMessage():
		case <-sigs:
			ipman.DeleteAllIP()
			_, ownMarkedIPs, _, err := dcs.GetIPs()
			if err != nil {
				log.Error("Cannot retrieve currently marked addresses from DCS for umarking")
			} else {
				dcs.UnMarkAllIPs(ownMarkedIPs)
			}
			return
		case <-time.After(time.Duration(conf.Interval) * time.Millisecond):
		}
	}
}

func cleanup(conf *config.Config, dcs dcs.Dcs, ipman ipmanager.IPManagerLocal) {
	registeredAddresses, err := ipman.GetAllIP()
	if err != nil {
		log.Error("encountered an error while checking registered addresses:")
		log.Error(err)
	}
	for _, address := range registeredAddresses {
		ip := address.IP.String()
		marked := dcs.CheckIpInDCS(ip)
		if !marked {
			err := ipman.DeleteIP(ip)
			if err != nil {
				log.Error("Failed to delete IP address: " + ip + " that I'm no longer supposed to use:")
				log.Error(err)
			}
		}
	}
}

func register(dcs dcs.Dcs, ipman ipmanager.IPManagerLocal) {
	dcs.AdvertiseInDCS()
	numAdv, err := dcs.GetNumberAdvertisments()
	if err != nil {
		log.Error("Error while retrieving number of advertising clients:")
		log.Error(err)
		return
	}
	log.Printf("There are %d clients advertising their healthiness.", numAdv)

	//GetNumberMarkedIPs will also refresh all "marked" keys that belong to this nodeName
	IPs, ownMarkedIPs, unmarkedIPs, err := dcs.GetIPs()
	numIps := len(IPs)
	numIpsOptimum := int(math.Ceil(float64(numIps) / float64(numAdv)))

	log.Printf("There are %d ip addresses that can be managed.", numIps)
	log.Printf("There are %d ip addresses managed by this yaim.", len(ownMarkedIPs))
	log.Printf("We should have %d ip addresses registered to this host.", numIpsOptimum)

	// if numIpsOptimum == 0 {
	// 	continue
	// }

	for i := 0; i < len(ownMarkedIPs); i++ {
		ip := ownMarkedIPs[i]
		if i < numIpsOptimum {
			//Check if the IP addresses marked are actually registered
			err := ipman.CheckIP(ip)
			if err != nil {
				log.Error("The marked IP: ", ip, " was not found to be registered locally.")
				log.Error("Removing mark from DCS.")
				dcs.UnMarkIpInDCS(ip)
				//dont refresh the mark.
				continue
			}
			//This will refresh the TTL of all previously marked IPs, except any superflous ones.
			//The TTL will indirectly "free" all
			//log.Debug("Trying to refresh mark for IP: " + ip + " in DCS")
			go dcs.RefreshMarkIpInDCS(ip)
		} else {
			//All remaining IPs will need to be removed.
			//When using Hetzner API, this is not necessary.
			err := ipman.DeleteIP(ip)
			if err != nil {
				log.Error("error while dropping IP: ", ip)
				log.Error(err)
			} else {
				log.Print("dropped IP: ", ip)
				dcs.UnMarkIpInDCS(ip)
			}
		}
	}

	if len(ownMarkedIPs) < numIpsOptimum {
		// We have too few IPs, try to register another one!
		// e.g. if 10 adresses and 3 yaim are available and they call this function all at the same time,
		// each one will try to register ceil(10/3)=4 adresses.
		// Only the first node will succeed in taking a fourth adress,
		// the other nodes will get an error from etcd.

		if len(unmarkedIPs) <= 0 {
			log.Print("we should take up more ip-addresses, but it seems like there are no ummarked ones.")
			log.Print("waiting for other yaim to release their mark or for the TTL to expire.")
			return
		}
		//select a random IP from unmarkedIPs
		rand.Seed(time.Now().Unix())
		ip := unmarkedIPs[rand.Intn(len(unmarkedIPs))]

		//try to mark the randomly select IP. True means we where successful in setting the etcd key.
		if dcs.MarkIpInDCS(ip) {
			err := ipman.AddIP(ip)
			if err != nil {
				log.Error("error while adding IP: ", ip, " :")
				log.Error(err)
				//TODO: think about deleting the key from the DCS straight away. Othwerwise, we'll need to wait for TTL to expire
			} else {
				log.Print("added IP: ", ip)
			}
		}
	}
}
