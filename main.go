package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/mdns"
)

func main() {
	var interfaceName string
	var port int

	flag.StringVar(&interfaceName, "interfaceName", "", "The network interface to expose")
	flag.IntVar(&port, "port", 0, "The port to expose")
	flag.Parse()

	if port == 0 {
		log.Println("port should be specified with --port")
		os.Exit(1)
	}
	if interfaceName == "" {
		log.Println("interfaceName should be specified with --interfaceName")
		os.Exit(1)
	}

	ip, err := findIPAddress(interfaceName)
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
	if ip == nil {
		log.Printf("Could not find an IP address (v4) for interface %s", interfaceName)
		os.Exit(1)
	}

	// Setup our service export
	host, _ := os.Hostname()
	info := []string{"Kcrypt challenger server"}
	service, _ := mdns.NewMDNSService(host, "_kcrypt._tcp", "", "", port, []net.IP{ip}, info)

	// Create the mDNS server, defer shutdown
	server, _ := mdns.NewServer(&mdns.Config{Zone: service})
	defer server.Shutdown()

	log.Printf("Server created. Advertising %s:%d as %s", ip, port, "_kcrypt._tcp")
	sitAndWait()
}

func sitAndWait() {
	// Create a channel to receive signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a channel to communicate with the goroutine
	exitChan := make(chan struct{})

	// Start a goroutine to listen for user input
	go listenForInput(exitChan)

	// Wait for a signal or user input to exit the program
	select {
	case sig := <-signalChan:
		fmt.Printf("Received signal: %v\n", sig)
	case <-exitChan:
		fmt.Println("User pressed a key. Exiting...")
	}

	// Perform cleanup or additional actions before exiting, if necessary
	fmt.Println("Program has exited.")
}

func listenForInput(exitChan chan<- struct{}) {
	fmt.Print("Press Enter to exit: ")

	// Create a scanner to read a line of input
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	// Send a signal to the main goroutine to exit
	exitChan <- struct{}{}
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
