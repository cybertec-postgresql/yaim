
## Start a local etcd cluster, e.g. using podman/docker:

```bash
[julian@fedora33-ws yaim]$ podman run -d --name etcd -p 2379:2379 -e "ETCD_ENABLE_V2=true" -e "ALLOW_NONE_AUTHENTICATION=yes" bitnami/etcd
5abcad1929ebd6c715374e97eb31d9921383afdecb78b4782b345b1268f143db
[julian@fedora33-ws yaim]$ curl -s http://127.0.0.1:2379/v2/keys/?recursive=true | jq
{
  "action": "get",
  "node": {
    "dir": true
  }
}
```

## Create a `yaim.yaml` config. In my case, I'm going to register addresses locally, with the `yaim` label. `yaim` is configured to check for a specific key and value in the DCS in this case:
```yaml
interval: 3000
ttl: 9000
retry_num: 2
retry_after: 250

dcs-namespace: "/service/"
dcs-clustername: "yaim"
dcs-type: etcd
dcs-endpoints:
  - http://127.0.0.1:2379

checker-type: http

interface: lo
netmask: 32
label: yaim

http-url: http://127.0.0.1:2379/v2/keys/test
http-expected-code: 200
http-expected-response-contains: '"value":"foo"'

log-level: Debug
```

## launch `yaim` (using sudo in this case, to enable yaim to add/remove IP addresses):
```bash
[julian@fedora33-ws yaim]$ sudo ./yaim --config yaim.yaml
[sudo] password for julian: 
INFO[0000] Using config from file: yaim.yaml            
INFO[0000] No nodename specified, instead using hostname: fedora33-ws 
INFO[0000] This is the config that will be used:        
	checker-type : http
	config : yaim.yaml
	dcs-clustername : yaim
	dcs-endpoints : [http://127.0.0.1:2379]
	dcs-namespace : /service/
	dcs-type : etcd
	hostingtype : basic
	http-expected-code : 200
	http-expected-response-contains : "value":"foo"
	http-url : http://127.0.0.1:2379/v2/keys/test
	interface : lo
	interval : 3000
	label : yaim
	log-level : Debug
	netmask : 32
	nodename : fedora33-ws
	retry-after : 250
	retry-num : 3
	retry_after : 250
	retry_num : 2
	ttl : 9000
	version : false
DEBU[0000] loop!                                        
DEBU[0000] The http health check query returned: {"errorCode":100,"message":"Key not found","cause":"/test","index":5} 
INFO[0000] Node is not healthy.                         
```

## If we create the key `yaim` is looking for, the output should change:
```bash
[julian@fedora33-ws yaim]$ curl -s -XPUT http://127.0.0.1:2379/v2/keys/test -d value=foo | jq
{
  "action": "set",
  "node": {
    "key": "/test",
    "value": "foo",
    "modifiedIndex": 6,
    "createdIndex": 6
  }
}
```

And the change in output by the still running `yaim`:
```bash
DEBU[0072] loop!                                        
DEBU[0072] The http health check query returned: {"errorCode":100,"message":"Key not found","cause":"/test","index":5} 
INFO[0072] Node is not healthy.                         
DEBU[0075] loop!                                        
DEBU[0075] The http health check query returned: {"action":"get","node":{"key":"/test","value":"foo","modifiedIndex":6,"createdIndex":6}} 
INFO[0075] Node is healthy.                             
INFO[0075] There are 1 clients advertising their healthiness. 
INFO[0075] There are 0 ip addresses that can be managed. 
INFO[0075] There are 0 ip addresses managed by this yaim. 
INFO[0075] We should have 0 ip addresses registered to this host. 
```

## Take a look at the resulting structure in etcd:
```bash
[julian@fedora33-ws yaim]$ curl -s http://127.0.0.1:2379/v2/keys/service/yaim/?recursive=true | jq
{
  "action": "get",
  "node": {
    "key": "/service/yaim",
    "dir": true,
    "nodes": [
      {
        "key": "/service/yaim/nodes",
        "dir": true,
        "nodes": [
          {
            "key": "/service/yaim/nodes/fedora33-ws",
            "value": "healthy",
            "expiration": "2021-06-21T08:56:44.060629204Z",
            "ttl": 8,
            "modifiedIndex": 39,
            "createdIndex": 39
          }
        ],
        "modifiedIndex": 34,
        "createdIndex": 34
      }
    ],
    "modifiedIndex": 34,
    "createdIndex": 34
  }
}
```

As you can see, we now have one node advertising their healthiness (since the /test key has the expected value for the defined check).

## Add an IP address to the pool in etcd and watch `yaim` detect the IP address and register it:
```bash
[julian@fedora33-ws yaim]$ curl -s http://127.0.0.1:2379/v2/keys/service/yaim/ips/127.0.0.11 -XPUT -d dir=true | jq
{
  "action": "set",
  "node": {
    "key": "/service/yaim/ips/127.0.0.11",
    "dir": true,
    "modifiedIndex": 90,
    "createdIndex": 90
  }
}
```

Output by `yaim`:
```bash
DEBU[0319] loop!                                        
DEBU[0319] The http health check query returned: {"action":"get","node":{"key":"/test","value":"foo","modifiedIndex":6,"createdIndex":6}} 
INFO[0319] Node is healthy.                             
INFO[0319] There are 1 clients advertising their healthiness. 
INFO[0319] There are 0 ip addresses that can be managed. 
INFO[0319] There are 0 ip addresses managed by this yaim. 
INFO[0319] We should have 0 ip addresses registered to this host. 
DEBU[0322] loop!                                        
DEBU[0322] The http health check query returned: {"action":"get","node":{"key":"/test","value":"foo","modifiedIndex":6,"createdIndex":6}} 
INFO[0322] Node is healthy.                             
INFO[0322] There are 1 clients advertising their healthiness. 
INFO[0322] There are 1 ip addresses that can be managed. 
INFO[0322] There are 0 ip addresses managed by this yaim. 
INFO[0322] We should have 1 ip addresses registered to this host. 
INFO[0322] marked IP in etcd: 127.0.0.11                
INFO[0322] Registered IP address: 127.0.0.11/32 lo:yaim 
INFO[0322] added IP: 127.0.0.11                         
DEBU[0325] loop!                                        
```

Check that the address has really been registered:
```bash
[julian@fedora33-ws yaim]$ ip address show lo
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet 127.0.0.11/32 scope global lo:yaim
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
```
> note the `yaim` label for the entry of the desired IP address `127.0.0.11`


Check etcd to see how `yaim` marked the ip address, including the TTL:
```bash
[julian@fedora33-ws yaim]$ curl -s http://127.0.0.1:2379/v2/keys/service/yaim/ips?recursive=true | jq
{
  "action": "get",
  "node": {
    "key": "/service/yaim/ips",
    "dir": true,
    "nodes": [
      {
        "key": "/service/yaim/ips/127.0.0.11",
        "dir": true,
        "nodes": [
          {
            "key": "/service/yaim/ips/127.0.0.11/marked",
            "value": "fedora33-ws",
            "expiration": "2021-06-21T09:02:58.273641323Z",
            "ttl": 8,
            "modifiedIndex": 232,
            "createdIndex": 228
          }
        ],
        "modifiedIndex": 226,
        "createdIndex": 226
      }
    ],
    "modifiedIndex": 226,
    "createdIndex": 226
  }
}
```

## Play around with:
    - adding more IP addresses to the pool in etcd
    - deleting/changing the `/test` key in etcd that is health-checked by `yaim`
    - starting more `yaim` (with different configurations, so that they use different names and labels if you're running them all locally, possibly also use different health checks)
    - delete an IP address from the pool