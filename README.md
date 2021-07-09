# yaim
yaim - yet another ip manager
yaim will check the healthiness of a component (e.g. via http GET, PostgreSQL query, shell).
If the node is healthy, this information is published in a DCS (distributed consensus store) and all yaim registered in the same service directory will race to take on virtual IP addresses.
All IP addresses specified in the DCS will always be connected to one of the nodes, so this concept can be easily used together with round robin DNS load balancing.
If the health check fails or the node goes down, the remaining nodes will shortly afterwards try to take over the unused IP addresses.

yaim can be used as a replacement for keepalived or other virtual IP management solutions.
It can provide high availability and redundancy on a network level and as such doesn't require any special client or server modifications.

## design
A running instance of this program will be referred to as a _node_.
In the DCS, the IP adresses must be added as keys in the `service/ips/` path.
Each IP address will be represented by a directory in DCS with the IP as the directory name. 

> The design of yaim is inspired by Patroni and vip-manager, so if you know either of those, yaim should seem familiar.

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

All configuration takes place in the `yaim.yml` file. `yaim` by default looks for the `yaim.yml` file in the same directory as it's being launched from, however a custom config location can be provided using the config flag.

### in yaim.yml
#### dcs-namespace
This is the namespace in the DCS in which all yaim clusters operate.
> This should be different from the namespace used by other applications to avoid conflicts.

### dcs-clustername
This is the directory in which this specific yaim cluster operates. This will be placed inside of the dcs-namespace directory.

#### interval
This is the main loop interval. After doing everything that is described in the design section, yaim will sleep for this many milliseconds.

#### ttl
The TTL that will be set for various keys. If the key expires, a failover would occur.

#### retry_num
Number of times yaim will try to get values from the etcd key-value store or try to ping the pgbouncer or postgresql database.
#### retry_after
Time to wait before trying to reach etcd or the database again.

#### dcs-type
The type of DCS used, currently only supports `etcd`

#### dcs-endpoints
A list of endpoints that can be used to access the same DCS cluster. The client will randomly try any of these endpoints.

#### etcd_user and etcd_password
Credentials to a user that may read and write within the dcs-namespace/dcs-clustername directory defined above.

#### checker-type: http
What kind of checker to use to evaluate healthiness.
Currently supports `http`.
Thinking of adding checkers for (PostgreSQL) databases and for running checks on the shell.

#### http-url
The URL to send the health check (GET request) to.

#### http-expected-code
What HTTP code implies healthiness? e.g. `200`

#### http-expected-response-contains
e.g. `'"value":"foo"'`


## usage

### adding IP addresses to the pool
simply create a directory with the name of the directory containing the ip-address in the KV-store, for example with etcd:
```
curl -s http://192.168.0.34:2379/v2/keys/service/yaim/ips/123.0.0.1 -XPUT -d dir=true
```
yaim will then register that a new IP is available and it will try _mark_ it.

### deleting addresses from the pool
This is just as easy as adding addresses, simply remove the directory from etcd:

```
curl -s http://192.168.0.34:2379/v2/keys/service/yaim/ips/123.0.0.1?recursive=true -XDELETE
```