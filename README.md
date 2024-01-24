This is a simple utility that takes 4 arguments:

- a network interface or an IP address
- a port
- a service type
- a hostname

It then advertises:
- the IPv4 address (or the one of the specified interface if one is found)
- the specified port
- the hostname

over mdns as service of the specified type. The hostname is included
in the response so that the client can identify this unique instance.

Example usage:

```
go run . --port 8000 --interfaceName enp121s0 --serviceType _kcrypt._tcp --hostName myserver.local
```

or

```
go run . --port 8000 --address 192.168.1.100 --serviceType _kcrypt._tcp --hostName myserver.local
```

In the context of [kcrypt-challenger](https://github.com/kairos-io/kcrypt-challenger),
this tool can be used to make a regular kcrypt challenger server be discoverable in
the local network. See original spike for more: https://github.com/kairos-io/kairos/issues/2069
