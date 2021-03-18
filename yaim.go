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
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/cybertec-postgresql/yaim/checker"
	"github.com/cybertec-postgresql/yaim/config"
	"github.com/cybertec-postgresql/yaim/dcs"
)

// var configFile = flag.String("config", "./yaim.yml", "Location of the configuration file.")
// var versionHint = flag.Bool("version", false, "Show the version number.")

var (
	// vip-manager version definition
	version string = "1.0.1"
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

	ipMan(conf, checker, dcs)
}

func ipMan(conf *config.Config, checker checker.Checker, dcs dcs.Dcs) {
	for {
		time.Sleep(time.Duration(conf.Interval) * time.Millisecond)
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
			dcs.AdvertiseInDCS()
			numAdv, err := dcs.GetNumberAdvertisments()
			if err != nil {
				log.Print("error while retrieving number of advertising clients.", err)
				continue
			}
			log.Printf("there are %d clients advertising their healthiness.", numAdv)

			//GetNumberMarkedIPs will also refresh all "marked" keys that belong to this nodeName
			numIps, ownMarkedIPs, unmarkedIPs, err := dcs.GetNumberMarkedIPs()
			numIpsOptimum := int(math.Ceil(float64(numIps) / float64(numAdv)))

			log.Printf("there are %d ip addresses that can be managed.", numIps)
			log.Printf("there are %d ip addresses managed by this yaim.", len(ownMarkedIPs))
			log.Printf("we should have %d ip addresses registered to this host.", numIpsOptimum)

			if numIpsOptimum == 0 {
				continue
			}

			for i := 0; i < len(ownMarkedIPs); i++ {
				if i < numIpsOptimum {
					//This will refresh the TTL of all previously marked IPs, except any superflous ones.
					//The TTL will indirectly "free" all
					go dcs.RefreshMarkIpInDCS(ownMarkedIPs[i])
				} else {
					//All remaining IPs will need to be removed.
					//When using Hetzner API, this is not necessary.
					//TODO: implement basic ip addr stuff.
					log.Print("dropped IP: ", ownMarkedIPs[i])
				}
			}

			if len(ownMarkedIPs) < numIpsOptimum {
				// We have too few IPs, try to register another one!
				// e.g. if 10 adresses and 3 yaim are available and they call this function all at the same time,
				// each one will try to register ceil(10/3)=4 adresses.
				// Only the first node will succeed in taking a fourth adress,
				// the other nodes will get an error from etcd.

				if len(unmarkedIPs) <= 0 {
					log.Print("we should take up more ip-addresses, but it seems like there are none in the array.")
					continue
				}
				//select a random IP from unmarkedIPs
				rand.Seed(time.Now().Unix())
				ip := unmarkedIPs[rand.Intn(len(unmarkedIPs))]

				//try to mark the randomly select IP. True means we where successful in setting the etcd key.
				if dcs.MarkIpInDCS(ip) {
					//use register the address to our machine.
					//TODO
					takeIp()
				}

			}
		} else {
			log.Print("Node is not healthy.")
			//TODO: make sure to drop addresses here.
			// we need to keep track of all registered virtual IP addresses, so we can drop them.
			// This will be useful to know when we stop the program as well.
		}
	}
}

func takeIp() {
	//TODO
	return
}
