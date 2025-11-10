This is a simple utility that can operate in two modes:

## Server Mode (default)

Advertises a service over mDNS. Takes 4 arguments:

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

## Lookup Mode

Queries for mDNS services on the network. Useful for discovering services advertised by the server mode.

Example usage (to discover the service advertised above):

```
go run . lookup --serviceType _kcrypt._tcp --interfaceName enp121s0 --timeout 10
```

or

```
go run . lookup --serviceType _kcrypt._tcp --timeout 10
```

The lookup command will output discovered services with their details:
- Name: The service instance name
- Host: The hostname of the service
- Port: The port number
- AddrV4: The IPv4 address
- Info: Service information/TXT records

**Note:** The `--interfaceName` flag is optional for lookup. If not specified, the system default multicast interface will be used. The `--timeout` flag defaults to 10 seconds if not specified.

In the context of [kcrypt-challenger](https://github.com/kairos-io/kcrypt-challenger),
this tool can be used to make a regular kcrypt challenger server be discoverable in
the local network. See original spike for more: https://github.com/kairos-io/kairos/issues/2069
