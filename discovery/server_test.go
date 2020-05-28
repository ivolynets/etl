package discovery

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {

	fmt.Println("Starting discovery server...")
	server := NewServer()

	go func() {

		err := server.Start(":9999")

		if err != nil {

			fmt.Fprintf(os.Stderr, "Could not start the discovery server: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("Discovery server started")

	}()

	addr, err := net.ResolveUDPAddr("udp", "255.255.255.255:9999")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	for i := 0; i < 10; i++ {
		n, err := conn.Write([]byte("Hello"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		fmt.Printf("write: %d\n", n)
		time.Sleep(time.Second)
	}

	conn.Close()
	server.Stop()
}
