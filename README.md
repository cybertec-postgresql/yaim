# yaim
yaim - yet another ip manager
yaim will check the healthiness of a specific postgresql or pgbouncer server. If the node is healthy, this information is published in etcd and all yaim registered in the same service directory will race to take on virtual IP addresses. All IP addresses specified in etcd will always lead to one of the nodes, so this concept can be easily used together with round robin DNS load balancing.

## design
A running instance of this program will be referred to as a _node_.
In the DCS, the IP adresses must be added as keys in the `service/ips/` path.
Each IP address will be represented by a directory in DCS with the IP as the directory name. 

The program runs in a loop:
```
for
  sleep(interval)
  if node is healthy {
    create a key in the dcs that advertises this node as being healthy.
      -the key has a TTL for expiry
      -the key is named according to the OS hostname
    (or refresh TTL if the entry already exists in the DCS) 
    
    check for ip addresses in DCS
    
    look up which addresses are marked and unmarked
    
    when looking at the number of healthy nodes and the number of ip addresses,
    to achieve roughly equal distribution among all nodes,
     - do we need to register more IP-addresses on our node's interface?
        - if so, then "mark" the ip address in dcs,
          - with a node with key name "marked" in the `service/ips/[address]/` directory
          - and a TTL for expiry
     - or do we have to drop some addresses?
        - then remove the ip-address from the interface.
        - key expiry will remove the "mark", so the ip address can then be taken by another node
    
    refresh the TTL of all "marked" IP addresses that belong to this node
  }
```
