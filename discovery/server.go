package discovery

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// TODO: move logging capabilities to separate package
var infoLog *log.Logger = log.New(os.Stdout, "discovery: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)
var errorLog *log.Logger = log.New(os.Stderr, "discovery: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)

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

	// initialize the server

	infoLog.Println("starting server...")
	if err := s.init(addr); err != nil {
		errorLog.Println(err)
		return err
	}
	infoLog.Println("server started")

	// listen to incoming requests

	buf := make([]byte, 1024)

	for {
		if req, ok, err := s.read(buf); !ok {
			break // server has been stopped
		} else if err == nil {

			n := 0
			if req != nil {
				n = len(req.data)
				s.buf <- req
			}

			infoLog.Printf("read_bytes = %d\n", n)

		} else {
			errorLog.Println(err)
			return err
		}
	}

	return nil
}

// Stop shuts down the discovery server.
func (s *Server) Stop() error {

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.conn == nil {
		msg := "server is not running"
		errorLog.Println(msg)
		return fmt.Errorf(msg)
	}

	infoLog.Println("stopping server...")

	close(s.buf)
	s.conn.Close()
	s.conn = nil

	infoLog.Println("server is stopped")
	return nil
}

// init performs initialization of the discovery server.
func (s *Server) init(addr string) error {

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.conn != nil {
		return fmt.Errorf("server is already running")
	}

	var udpAddr *net.UDPAddr
	var err error

	if udpAddr, err = net.ResolveUDPAddr("udp", addr); err != nil {
		return err
	}

	if s.conn, err = net.ListenUDP("udp", udpAddr); err != nil {
		return err
	}

	// TODO: make queue size configurable
	s.buf = make(chan *Request)

	// go routine for datagrams handling

	go func() {
		for {
			if req, ok := <-s.buf; ok {
				infoLog.Printf("addr = %s, data = %s\n", req.addr, req.data)
			} else {
				// server is stopped
				return
			}
		}
	}()

	return nil
}

// read reads the request from the connection. This function is timing out if
// no bytes have been received during past second, in this case the nil
// reference to request is returned. The second return parameter is always
// true unless the server was stopped during the last attempt and connection
// was closed.
func (s *Server) read(buf []byte) (*Request, bool, error) {

	var n int
	var addr *net.UDPAddr
	var err error

	s.mux.Lock()
	defer s.mux.Unlock()

	if s.conn == nil {
		return nil, false, nil
	}

	// TODO: make read timeout configurable
	if err = s.conn.SetReadDeadline(time.Now().Add(time.Second)); err == nil {
		n, addr, err = s.conn.ReadFromUDP(buf)
	}

	if err != nil {
		// ignore timeout error
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			return nil, true, err
		}
	}

	if n > 0 {
		data := make([]byte, n)
		copy(data, buf[0:n])
		return &Request{addr, string(data)}, true, nil
	}

	return nil, true, nil
}
