package main

// yaim: 	yet another ip manager
//
// written:	by Julian Markwort in 2018/2019 at Cybertec Schönig & Schönig GmbH.
// Mail:	julian.markwort@cybertec.at
// Mail: 	office@cybertec.at
// Website:	www.cybertec-postgresql.com

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"go.etcd.io/etcd/client"
)

var configFile = flag.String("config", "./yaim.yml", "Location of the configuration file.")
var versionHint = flag.Bool("version", false, "Show the version number.")

//start all fields uppercase so yaml package can access them
type conf struct {
	Interval uint64 `yaml:interval` //milliseconds
	TTL      uint64 `yaml:interval` //milliseconds - should be at least 2*RTT greater than Interval (once to retrieve marked IPs, once to refresh all marks.).

	Retry_after int `yaml:retry_after`
	Retry_num   int `yaml:retry_num`

	Endpoints     []string `yaml:enpoints`
	Etcd_user     string   `yaml:etcd_user`
	Etcd_password string   `yaml:etcd_password`

	Service string `yaml:service`

	Pgbouncer_address  string `yaml:pgbouncer_address`
	Pgbouncer_port     string `yaml:pgbouncer_port`
	Pgbouncer_database string `yaml:pgbouncer_database`
	Pgbouncer_user     string `yaml:pgbouncer_user`
	Pgbouncer_password string `yaml:pgbouncer_password`
	Db_options         string `yaml:db_options`
}

var c *conf
var kapi client.KeysAPI

var getOpts *client.GetOptions
var getRecursiveOpts *client.GetOptions

var ttlSetOpts *client.SetOptions
var dirSetOpts *client.SetOptions

var nodeName string

func main() {
	flag.Parse()

	if *versionHint == true {
		fmt.Println("version 0.1")
		return
	}

	c = new(conf)

	yamlFile, yamlErr := ioutil.ReadFile(*configFile)
	if yamlErr != nil {
		log.Fatal("couldn't open config File!", yamlErr)
	}
	yamlErr = yaml.Unmarshal(yamlFile, c)
	if yamlErr != nil {
		log.Fatalf("Unmarshal: %v", yamlErr)
	}
	fmt.Println("read config:")
	fmt.Println(*c)

	name, nameErr := os.Hostname()
	if nameErr != nil {
		log.Fatal(nameErr)
	}
	nodeName = name

	cfg := client.Config{
		Endpoints: c.Endpoints,
		Transport: client.DefaultTransport,
		//HeaderTimeoutPerRequest: time.Second,
		Username: c.Etcd_user,
		Password: c.Etcd_password,
	}

	cl, err := client.New(cfg)
	if err != nil {
		log.Fatal("couldn't initialize etcd client", err)
	}
	kapi = client.NewKeysAPI(cl)

	getRecursiveOpts = &client.GetOptions{
		Recursive: true,
		Sort:      true,
		Quorum:    true,
	}
	getOpts = &client.GetOptions{
		Recursive: false,
		Sort:      true,
		Quorum:    true,
	}

	ttlSetOpts = &client.SetOptions{
		TTL: time.Duration(c.TTL) * time.Millisecond,
	}
	dirSetOpts = &client.SetOptions{
		Dir:       true,
		PrevExist: client.PrevIgnore,
	}

	_, dirErr := kapi.Get(context.Background(), c.Service+"nodes", getOpts)
	if dirErr != nil {
		_, dirErr = kapi.Set(context.Background(), c.Service+"nodes", "", dirSetOpts)
		if dirErr != nil {
			log.Fatal("couldn't create nodes dir in etcd.", dirErr)
		}
	}
	_, dirErr = kapi.Get(context.Background(), c.Service+"ips", getOpts)
	if dirErr != nil {
		_, dirErr = kapi.Set(context.Background(), c.Service+"ips", "", dirSetOpts)
		if dirErr != nil {
			log.Fatal("couldn't create ips dir in etcd.", dirErr)
		}
	}

	ipMan()
}

func ipMan() {
	for {
		time.Sleep(time.Duration(c.Interval) * time.Millisecond)
		fmt.Println("loop!")

		var err error
		var healthy bool
		for i := 0; i < c.Retry_num; i++ {
			healthy, err = isHealthy()
			if err != nil {
				log.Printf("encountered an error while determining health status.\n")
				log.Print(err)
			} else {
				break
			}
			time.Sleep(time.Duration(c.Retry_after) * time.Millisecond)
		}
		if err != nil {
			log.Print("too many retries")
		}

		if healthy == true {
			log.Print("Node is healthy.")
			advertiseInDCS()
			numAdv, err := getNumberAdvertisments()
			if err != nil {
				log.Print("error while retrieving number of advertising clients.", err)
				continue
			}
			log.Printf("there are %d clients advertising their healthiness.", numAdv)

			//getNumberMarkedIPs will also refresh all "marked" keys that belong to this nodeName
			numIps, ownMarkedIPs, unmarkedIPs, err := getNumberMarkedIPs()
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
					go refreshMarkIpInDCS(ownMarkedIPs[i])
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
				if markIpInDCS(ip) {
					//use register the address to our machine.
					//TODO
					takeIp()
				}

			}
		} else {
			log.Print("Node is not healthy.")
		}
	}
}

func isHealthy() (success bool, err error) {
	connstr := "postgres://" + c.Pgbouncer_user + ":" + c.Pgbouncer_password
	connstr += "@" + c.Pgbouncer_address + ":" + c.Pgbouncer_port
	connstr += "/" + c.Pgbouncer_database + c.Db_options

	db, dbErr := sql.Open("postgres", connstr)

	if dbErr != nil {
		return false, dbErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	dbErr = db.PingContext(ctx)
	if dbErr != nil {
		return false, dbErr
	} else {
		return true, nil
	}
}

func takeIp() {
	//TODO
	return
}

func advertiseInDCS() {
	//create key for this node in the DCS, if it exists this will simply update the TTL.
	_, err := kapi.Set(context.Background(), c.Service+"nodes/"+nodeName, "healthy", ttlSetOpts)
	if err != nil {
		log.Print(err)
	}
}

func markIpInDCS(ip string) (success bool) {
	opts := &client.SetOptions{
		PrevExist: client.PrevNoExist,
		TTL:       time.Duration(c.TTL) * time.Millisecond,
	}

	//create "marked" key for this node in the directory of ip in DCS
	_, err := kapi.Set(context.Background(), c.Service+"ips/"+ip+"/marked", nodeName, opts)
	if err != nil {
		log.Print("Error in markIpInDCS() :", err)
		return false
	} else {
		log.Print("marked IP in etcd: ", ip)
	}
	return true
}

func refreshMarkIpInDCS(ip string) {
	opts := &client.SetOptions{
		PrevValue: nodeName,
		TTL:       time.Duration(c.TTL) * time.Millisecond,
		Refresh:   true,
	}

	//refresh "marked" key for this node in the directory of ip in DCS, only if the value (nodeName) is "ours".
	_, err := kapi.Set(context.Background(), c.Service+"ips/"+ip+"/marked", "", opts)
	if err != nil {
		log.Print("Error in refreshMarkIpInDCS() :", err)
	} else {
		log.Print("refreshed marked IP in etcd: ", ip)
	}
}

func unMarkIpInDCS(ip string) {
	opts := &client.DeleteOptions{
		PrevValue: nodeName,
	}

	//remove "marked" key for this node in the directory of ip in DCS, only if the value (nodeName) is "ours".
	_, err := kapi.Delete(context.Background(), c.Service+"ips/"+ip+"/marked", opts)
	if err != nil {
		log.Print("Error in unMarkIpInDCS() :", err)
	}
	log.Print("removed mark for IP in etcd: ", ip)
}

func getNumberAdvertisments() (num int, err error) {
	//retrieve all advertised nodes
	resp, err := kapi.Get(context.Background(), c.Service+"nodes", getRecursiveOpts)
	if err == nil {
		if resp.Node.Dir {
			return len(resp.Node.Nodes), nil
		} else {
			err = errors.New("No advertisments of any nodes (including my own, apparently) where found.")
		}
	}
	return -1, err
}

func getNumberMarkedIPs() (numIps int, ownMarkedIPs, unmarkedIPs []string, err error) {
	//retrieve all ips
	resp, err := kapi.Get(context.Background(), c.Service+"ips", getRecursiveOpts)
	if err != nil {
		return 0, nil, nil, err
	}
	if resp.Node.Dir == false {
		err = errors.New("The \"" + c.Service + "ips\" path was no directory!")
		return 0, nil, nil, err
	}
	numIps = len(resp.Node.Nodes)
	for _, n := range resp.Node.Nodes {
		ip := strings.TrimPrefix(n.Key, c.Service+"ips/")
		if n.Dir {
			for _, nn := range n.Nodes {
				//If the first entry in the directory of this ip has a key of "marked", we'll count it as this IP being used by any yaim.
				if strings.TrimPrefix(nn.Key, c.Service+"ips/"+ip+"/") == "marked" {
					log.Print("marked value found!")
					//If the first entry in the directory of this ip has a value of our own nodeName, we'll count it as this IP being used by _this_ yaim.
					if nn.Value == nodeName {
						log.Print("our own marked value found!")
						ownMarkedIPs = append(ownMarkedIPs, strings.TrimPrefix(n.Key, c.Service+"ips/"))
					}
				}
			}
			if len(n.Nodes) <= 0 {
				//IP not marked!
				unmarkedIPs = append(unmarkedIPs, strings.TrimPrefix(n.Key, c.Service+"ips/"))
			}
		}
	}
	return numIps, ownMarkedIPs, unmarkedIPs, err
}
