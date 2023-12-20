package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/mdns"
)

func main() {
	// Setup our service export
	host, _ := os.Hostname()
	info := []string{"Kcrypt challenger server"}
	//ips := []net.IP{net.ParseIP("192.168.1.44")}
	ips, err := IPs()
	fmt.Printf("ips = %+v\n", ips)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		os.Exit(1)
	}

	service, _ := mdns.NewMDNSService(host, "_kcrypt._tcp", "", "", 8000, ips, info)

	// Create the mDNS server, defer shutdown
	server, _ := mdns.NewServer(&mdns.Config{Zone: service})
	defer server.Shutdown()

	fmt.Println("Server created. Will sit and do nothing now.")
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

func IPs() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{}, err
	}

	result := []net.IP{}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		fmt.Printf("name = %+v\n", iface.Name)
		fmt.Printf("addrs = %+v\n", addrs)
		if err != nil {
			fmt.Println("  Error getting addresses:", err)
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				fmt.Println("    Error parsing address:", err)
				continue
			}

			// Check if it's an IPv4 address
			if ipv4 := ip.To4(); ipv4 != nil {
				result = append(result, ipv4)
			}
		}
	}
	return result, nil
}
