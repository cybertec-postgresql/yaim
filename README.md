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

## configuration

All configuration takes place in the yaim.yml file. pgbouncer_man by default looks for the yaim.yml file in the same directory as it's being launched from, however a custom config location can be provided using the config flag.

### in yaim.yml
#### service
This is the namespace in which yaim operates. This must be different from the namespace used by your patroni cluster (and thus, also different from the namespace used in pgbouncer_man), to avoid conflicts.

#### interval
This is the main loop interval. After doing everything that is described in the design section, yaim will sleep for this amount of milliseconds.

#### retry_num
Number of times yaim will try to get values from the etcd key-value store or try to ping the pgbouncer or postgresql database.
#### retry_after
Time to wait before trying to reach etcd or the database again.

#### etcd_endpoints
A list of endpoints that can be used to access the same etcd cluster. The client will randomly try any of these endpoints.

#### etcd_user and etcd_password
Credentials to a user that may readingly access the contents within the `service` directory defined above.

#### pgbouncer_*
These settings are used to connect to the `pgbouncer` table that is provided by pgbouncer itself. These settings must point to the pgbouncer running on the same host as this yaim.

#### db_options
URL-style option notation for the connection that will be used to read from `pg_database`.


## usage

### adding IP addresses to the pool
simply create a folder with the name containing the ip-address in the KV-store, for example with etcd:
```
curl -s --basic --user patroni:UpiU178oURwaK4RQ7Gw  http://192.168.0.34:2379/v2/keys/yaim_service/ips/123.0.0.1 -XPUT -d dir=true
```
yaim will then register that a new IP is available and it will try _mark_ it.

### deleting addresses from the pool
This is just as easy as adding addresses, simply remove the directory from etcd:

```
curl -s --basic --user patroni:UpiU178oURwaK4RQ7Gw  http://192.168.0.34:2379/v2/keys/yaim_service/ips/123.0.0.1?recursive=true -XDELETE
```