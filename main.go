package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/miekg/dns"
)

func main() {
	if len(os.Args) < 2 {
		runServer()
		return
	}

	switch os.Args[1] {
	case "lookup", "query":
		runLookup()
	default:
		runServer()
	}
}

func runServer() {
	var interfaceName string
	var address string
	var serviceType string
	var hostname string
	var port int
	var bindToInterface bool
	var err error
	var ip net.IP

	flagSet := flag.NewFlagSet("server", flag.ExitOnError)
	flagSet.StringVar(&hostname, "hostName", "", "The hostname that uniquely identifies this instance")
	flagSet.StringVar(&interfaceName, "interfaceName", "", "The network interface to expose")
	flagSet.StringVar(&address, "address", "", "The IP address to advertise")
	flagSet.StringVar(&serviceType, "serviceType", "", "The type to advertise over mdns (e.g. \"_kcrypt._tcp\")")
	flagSet.IntVar(&port, "port", 0, "The port to expose")
	flagSet.BoolVar(&bindToInterface, "bindToInterface", true, "Bind to specific interface (set false for VM/bridge scenarios)")

	// Parse flags, handling both cases: with or without subcommand
	if len(os.Args) > 1 && os.Args[1] != "lookup" && os.Args[1] != "query" {
		flagSet.Parse(os.Args[1:])
	} else if len(os.Args) > 2 {
		flagSet.Parse(os.Args[2:])
	} else {
		flagSet.Parse(os.Args[1:])
	}

	if port == 0 {
		log.Println("port should be specified with --port")
		os.Exit(1)
	}
	if interfaceName == "" && address == "" {
		log.Println("interfaceName or address should be specified (--interfaceName or --address)")
		log.Println("Note: If you want to use system default interface, specify --address with")
		log.Println("a reachable IP and --bindToInterface=false")
		os.Exit(1)
	}

	if serviceType == "" {
		log.Println("serviceType should be specified with --serviceType")
		os.Exit(1)
	}

	if hostname == "" {
		log.Println("hostName should be specified with --hostName")
		os.Exit(1)
	}

	// Create a valid FQDN from the hostname
	if !strings.HasSuffix(hostname, ".") {
		hostname += "."
	}

	var iface *net.Interface

	if address != "" {
		ip = net.ParseIP(address)
		if ip == nil {
			log.Println("invalid IPv4 address specified")
			os.Exit(1)
		}
		if bindToInterface {
			// Find the interface that has this IP address to bind multicast to it
			iface, err = findInterfaceByIP(ip)
			if err != nil {
				log.Printf("Warning: Error finding interface for IP %s: %v. Using system default.", ip, err)
				iface = nil
			} else if iface == nil {
				log.Printf("Warning: Could not find interface for IP %s. Using system default.", ip)
			} else {
				log.Printf("Found interface %s for IP %s", iface.Name, ip)
			}
		} else {
			log.Printf("Using IP %s for advertisement but not binding to specific interface", ip)
			iface = nil
		}
	} else {
		ip, err = findIPAddress(interfaceName)
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
		if ip == nil {
			log.Printf("Could not find an IP address (v4) for interface %s", interfaceName)
			os.Exit(1)
		}
		if bindToInterface {
			// Get the interface by name to bind multicast to it
			iface, err = net.InterfaceByName(interfaceName)
			if err != nil {
				log.Printf("Warning: Error finding interface %s: %v. Using system default.", interfaceName, err)
				iface = nil
			} else {
				log.Printf("Using interface %s", iface.Name)
			}
		} else {
			log.Printf("Using IP %s from interface %s but not binding to specific interface", ip, interfaceName)
			iface = nil
		}
	}

	// Setup our service export
	// instance: unique name for this service instance
	// service: service type (e.g., "_kcrypt._tcp")
	// domain: DNS domain (empty = "local")
	// hostName: hostname (empty = use system hostname)
	// port: service port
	// ips: IP addresses to advertise
	// txt: TXT record data
	instanceName := strings.TrimSuffix(hostname, ".")

	info := []string{"An instance of " + serviceType}
	baseService, err := mdns.NewMDNSService(instanceName, serviceType, "", "", port, []net.IP{ip}, info)
	if err != nil {
		log.Printf("Error creating mDNS service: %v", err)
		os.Exit(1)
	}

	// Wrap the service with a logging zone to track queries
	service := &loggingZone{
		zone: baseService,
	}

	// Create the mDNS server, defer shutdown
	// Set Iface if we found one, otherwise let the library choose
	// NOTE: The hashicorp/mdns library silently ignores errors when binding to multicast.
	// If binding fails (e.g., port 5353 already in use), the server will be created
	// but won't actually listen. We enable LogEmptyResponses to help debug.
	//
	// IMPORTANT: mDNS Response Behavior
	// - Queries are typically multicast to 224.0.0.251:5353
	// - Responses can be either UNICAST (directly to client IP) or MULTICAST (to 224.0.0.251:5353)
	// - The hashicorp/mdns library follows RFC 6762: responses are typically UNICAST if the
	//   query came from a unicast address, MULTICAST if from multicast
	// - UNICAST responses may not route correctly across VM boundaries or network segments
	// - Binding to a specific interface limits responses to that interface only
	//
	// If clients aren't receiving responses:
	// 1. Try --bindToInterface=false to respond on all interfaces
	// 2. Use tcpdump/wireshark to verify: "sudo tcpdump -i any port 5353"
	// 3. Check if responses are unicast (to client IP) vs multicast (224.0.0.251)
	config := &mdns.Config{
		Zone:              service,
		LogEmptyResponses: true, // Enable logging to debug queries (logs when no response available)
	}
	if iface != nil {
		config.Iface = iface
		log.Printf("Binding mDNS server to interface %s (IP: %s)", iface.Name, ip)
		log.Printf("WARNING: Binding to a specific interface may prevent responses from reaching")
		log.Printf("clients on other network segments (e.g., VMs). If queries aren't working,")
		log.Printf("try running without --address/--interfaceName to use system default.")
	} else {
		log.Printf("Using system default multicast interface (will respond on all interfaces)")
	}
	server, err := mdns.NewServer(config)
	if err != nil {
		log.Printf("Error creating mDNS server: %v", err)
		log.Printf("NOTE: If port 5353 is already in use (e.g., by avahi-daemon),")
		log.Printf("the server may fail to bind. Consider stopping avahi-daemon or")
		log.Printf("using a different mDNS implementation.")
		os.Exit(1)
	}
	defer server.Shutdown()

	log.Printf("Server created. Advertising %s:%d as %s of type %s", ip, port, hostname, serviceType)
	log.Printf("Service will respond to queries for: %s", serviceType+".local.")
	log.Printf("NOTE: The server only responds to queries - it does not proactively announce.")
	log.Printf("Make sure clients query for: %s", serviceType+".local.")
	log.Printf("")
	log.Printf("DEBUGGING: If clients aren't receiving responses, check:")
	log.Printf("  - Responses may be UNICAST (to client IP) which may not route across VM boundaries")
	log.Printf("  - Use: sudo tcpdump -i any port 5353 to see mDNS traffic")
	log.Printf("  - Try: --bindToInterface=false to respond on all interfaces")
	log.Printf("  - Check FIREWALL rules on both host AND client (VM) - unicast UDP may be blocked")
	log.Printf("")
	sitAndWait()
}

func sitAndWait() {
	// Create a channel to receive signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for a signal to exit
	<-signalChan
	fmt.Println("Received signal. Shutting down...")
}

func runLookup() {
	var serviceType string
	var interfaceName string
	var timeout int

	flagSet := flag.NewFlagSet("lookup", flag.ExitOnError)
	flagSet.StringVar(&serviceType, "serviceType", "", "The service type to lookup (e.g., \"_kcrypt._tcp\")")
	flagSet.StringVar(&interfaceName, "interfaceName", "", "The network interface to use for the lookup")
	flagSet.IntVar(&timeout, "timeout", 10, "Timeout in seconds for the lookup")

	if len(os.Args) > 2 {
		flagSet.Parse(os.Args[2:])
	} else {
		flagSet.Parse(os.Args[1:])
	}

	if serviceType == "" {
		log.Println("serviceType must be specified with --serviceType")
		os.Exit(1)
	}

	var iface *net.Interface
	var err error

	if interfaceName != "" {
		iface, err = net.InterfaceByName(interfaceName)
		if err != nil {
			log.Printf("Error finding interface %s: %v", interfaceName, err)
			os.Exit(1)
		}
		fmt.Printf("Using interface: %s\n", iface.Name)
	}

	// Make a channel for results and start listening
	entriesCh := make(chan *mdns.ServiceEntry, 4)
	go func() {
		for entry := range entriesCh {
			fmt.Printf("Got new entry:\n")
			fmt.Printf("  Name: %s\n", entry.Name)
			fmt.Printf("  Host: %s\n", entry.Host)
			fmt.Printf("  Port: %d\n", entry.Port)
			fmt.Printf("  AddrV4: %v\n", entry.AddrV4)
			fmt.Printf("  AddrV6: %v\n", entry.AddrV6)
			fmt.Printf("  Info: %s\n", entry.Info)
			fmt.Printf("  InfoFields: %v\n", entry.InfoFields)
			fmt.Println()
		}
	}()

	fmt.Printf("Looking up service: %s\n", serviceType)
	fmt.Printf("Timeout: %d seconds\n", timeout)
	fmt.Println()

	// Use Query with interface if specified
	params := mdns.DefaultParams(serviceType)
	if iface != nil {
		params.Interface = iface
	}
	params.Entries = entriesCh
	params.Timeout = time.Duration(timeout) * time.Second
	params.DisableIPv6 = true // Disable IPv6 to avoid "network is unreachable" errors
	err = mdns.Query(params)

	if err != nil {
		log.Printf("Error during lookup: %v", err)
		os.Exit(1)
	}

	// Wait for results
	time.Sleep(time.Duration(timeout) * time.Second)
	close(entriesCh)

	fmt.Println("Lookup completed.")
}

func findIPAddress(iName string) (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Name == iName {
			addrs, err := iface.Addrs()
			if err != nil {
				return nil, fmt.Errorf("error getting addresses: %w", err)
			}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					return nil, fmt.Errorf("parsing address: %w", err)
				}

				// Check if it's an IPv4 address
				if ipv4 := ip.To4(); ipv4 != nil {
					return ipv4, nil
				}
			}
		}
	}

	return nil, nil
}

func findInterfaceByIP(targetIP net.IP) (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for i := range interfaces {
		iface := &interfaces[i]
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}

			// Check if it's an IPv4 address and matches
			if ipv4 := ip.To4(); ipv4 != nil && ipv4.Equal(targetIP) {
				return iface, nil
			}
		}
	}

	return nil, nil
}

// loggingZone wraps an mdns.Zone to log incoming queries
// Note: The Zone interface doesn't expose client IP, so we log the query details.
// To see client IPs, use: sudo tcpdump -i any port 5353
type loggingZone struct {
	zone mdns.Zone
}

func (lz *loggingZone) Records(q dns.Question) []dns.RR {
	// Log the incoming query
	log.Printf("Received query: %s (type: %d, class: %d)", q.Name, q.Qtype, q.Qclass)

	// Get records from the underlying zone
	records := lz.zone.Records(q)

	if len(records) > 0 {
		log.Printf("  -> Responding with %d record(s) for query: %s", len(records), q.Name)
		log.Printf("  -> NOTE: Response will be sent as UNICAST to client IP (per RFC 6762)")
		log.Printf("  -> UNICAST responses may be blocked by VM/client firewall - check firewall rules!")
		log.Printf("  -> To see actual client IP and response destination, use: sudo tcpdump -i any port 5353")
	} else {
		log.Printf("  -> No records found for query: %s", q.Name)
	}

	return records
}
