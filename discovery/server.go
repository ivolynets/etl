package discovery

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Server serves requests from other nodes in the grid.
type Server struct {
	conn *net.UDPConn
	buf  chan *Request
	mux  sync.Mutex
}

// Request represents a request for quotes from the other nodes.
type Request struct {
	addr *net.UDPAddr
	data string
}

// NewServer initializes a new discobvery server instance and returns a pointer
// to it.
func NewServer() *Server {
	return &Server{}
}

// Start launches the discovery server which is listening to the address and
// port specified in addr. This is a blocking method, if you want the server to
// be started in the background then call this method in separate goroutine. In
// this case don't forget to defer Stop method call.
func (s *Server) Start(addr string) error {

	err := s.init(addr)
	if err != nil {
		return err
	}

	buf := make([]byte, 1024)

	for {
		err := s.read(buf)
		if err != nil {
			return err
		}
	}

}

// Stop shuts down the discovery server.
func (s *Server) Stop() error {

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.conn == nil {
		return fmt.Errorf("Server is not running")
	}

	close(s.buf)
	s.conn.Close()
	s.conn = nil

	return nil
}

// init performs initialization of the discovery server.
func (s *Server) init(addr string) error {

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.conn != nil {
		return fmt.Errorf("Server is already running")
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	// TODO: make queue size configurable
	s.buf = make(chan *Request)

	// go routine for datagrams handling

	go func() {

		for {
			req, ok := <-s.buf
			if !ok {
				// server is stopped
				return
			}

			fmt.Printf("%s: %s\n", req.addr, req.data)
		}
	}()

	return nil
}

// read reads the datagram and sends it to the handler.
func (s *Server) read(buf []byte) error {

	var n int
	var addr *net.UDPAddr

	s.mux.Lock()
	defer s.mux.Unlock()

	// TODO: make read timeout configurable
	err := s.conn.SetReadDeadline(time.Now().Add(time.Second))
	if err == nil {
		n, addr, err = s.conn.ReadFromUDP(buf)
	}

	if err != nil {
		// ignore timeout error
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			return err
		}
	}

	fmt.Printf("read: %d\n", n)
	s.buf <- &Request{addr, string(buf[0:n])}

	return nil
}
