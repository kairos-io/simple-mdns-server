This is a simple utility that takes 3 arguments:

- a network interface
- a port
- a service type

It then advertises the IPv4 address of that interface (if one is found) and the
specified port, over mdns as service of the specified type.

Example usage:

```
go run . --port 8000 --interfaceName enp121s0 --serviceType _kcrypt._tcp
```

In the context of [kcrypt-challenger](https://github.com/kairos-io/kcrypt-challenger),
this tool can be used to make a regular kcrypt challenger server be discoverable in
the local network. See original spike for more: https://github.com/kairos-io/kairos/issues/2069
