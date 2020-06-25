package discovery

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	_net "github.com/ivolynets/etl/internal/net"
)

// TODO: move logging capabilities to separate package

// Info level logger which outputs messages to the system standard output.
var serverInfoLog *log.Logger = log.New(os.Stdout, "discovery_server: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)

// Error level logger which outputs messages to the system error output.
var serverErrorLog *log.Logger = log.New(os.Stderr, "discovery_server: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)

// server encapsulates the structure of the server to protect its internal
// state from explicit manipulation.
type server struct {
	conn _net.UDPConn  // UDP connection
	buf  chan *Request // input requests buffer
	ctl  chan byte     // control channel used for goroutines synchronization
}

// Server serves discovery requests from other nodes in the grid.
type Server struct {
	server
}

// StartServer launches the discovery server which is listening to the given
// UDP port.
//
// This is a non-blocking method which runs the server in a separate goroutine.
// In order to shutdown the server please call Stop() method.
//
// Please note, the server does not allow to use either system or dynamic port
// numbers, only the range of port numbers from 1024 through 49151 are allowed
// for use.
func StartServer(port int) (*Server, error) {

	// validate input
	if port < 1024 || port > 49151 {
		return nil, errors.New("the use of system or dynamic ports is prohibited")
	}

	// initialize the server

	serverInfoLog.Printf("starting server on port %d ...\n", port)
	var caddr *net.UDPAddr
	var conn _net.UDPConn
	var err error

	if caddr, err = _net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port)); err != nil {
		return nil, err
	}

	if conn, err = _net.ListenUDP("udp", caddr); err != nil {
		return nil, err
	}

	// TODO: make channel buffer size configurable if needed
	s := &Server{server{
		conn: conn,
		buf:  make(chan *Request),
		ctl:  make(chan byte),
	}}

	// start main and worker goroutines

	var wg sync.WaitGroup
	wg.Add(1)

	go s.handleRequests(&wg)
	go s.run(&wg)

	serverInfoLog.Println("server started")
	return s, nil
}

// Stop shuts down the discovery server.
func (s *Server) Stop() error {
	serverInfoLog.Println("stopping server ...")
	s.ctl <- 0
	return nil
}

// run is listening to incoming requests and puts them into the input channel
// for further processing. This method blocks until the Close() method is
// called.
func (s *Server) run(wg *sync.WaitGroup) {

	defer s.conn.Close() // close connection when the server is stopped

	// listen to incoming requests

	buf := make([]byte, 256) // TODO: revise buffer size or make it configurable

	for {
		select {
		case <-s.ctl:

			// stop goroutines

			close(s.buf) // close input channel for writes
			wg.Wait()    // wait until backlog is drained

			serverInfoLog.Println("server is stopped")
			return

		default:
			if n, addr, err := s.read(buf); err == nil {
				if n > 0 {
					serverInfoLog.Printf("read_bytes = %d, addr = %s\n", n, addr)
					var req *Request
					if req, err = deserialize(buf[0:n], addr); err == nil {
						s.buf <- req
					} else {
						serverErrorLog.Println(err)
					}
				}
			} else {
				serverErrorLog.Println(err)
			}
		}
	}
}

// read reads the request from the connection. It populates received data into
// the given buffer and returns a number of received bytes as a first parameter.
// This function is timing out if no bytes have been received during the past
// second, in this case the 0 received bytes is returned.
func (s *Server) read(buf []byte) (int, *net.UDPAddr, error) {

	// validate input

	if len(buf) == 0 {
		return 0, nil, errors.New("read buffer cannot be nil or of 0 size")
	}

	// read datagram

	var n int
	var addr *net.UDPAddr
	var err error

	// TODO: make read timeout configurable
	if err = s.conn.SetReadDeadline(time.Now().Add(time.Second)); err == nil {
		if n, addr, err = s.conn.ReadFromUDP(buf); err != nil {
			// ignore timeout error
			if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
				return 0, addr, err
			}
		}
		// please note that according to documentation even though timeout
		// occurred the portion of bytes still might have been read
		return n, addr, nil
	}

	return 0, nil, err
}

// write sends a message to the destination UDP address and returns a number of
// bytes sent.
func (s *Server) write(data []byte, addr *net.UDPAddr) (n int, err error) {

	// validate input

	if len(data) == 0 {
		return 0, errors.New("cannot send empty data")
	}

	if addr == nil {
		return 0, errors.New("unknown destination address")
	}

	// send message

	if n, err = s.conn.WriteToUDP(data, addr); err == nil {
		serverInfoLog.Printf("sent_bytes = %d, addr = %s\n", n, addr)
	}

	return
}

// handleRequests processes all the inquiries which are coming from other nodes.
// It is running in separate goroutine and pushing results into the output
// channel so that another goroutine handleResponses could send reply back to
// the caller.
func (s *Server) handleRequests(wg *sync.WaitGroup) {
	for req := range s.buf {

		serverInfoLog.Printf("request = %s, addr = %s, data = %s\n", ops[req.inq], req.addr, req.payload)

		if req.inq == Dnop {
			// the response is not expected for NOP inquiries
			continue
		}

		// create response

		res := &Response{req.inq, req.addr, req.payload}
		serverInfoLog.Printf("response = %s, addr = %s, data = %s\n", ops[res.inq], res.addr, res.payload)

		// serialize and send response

		var data []byte
		var addr *net.UDPAddr
		var err error
		if data, addr, err = serialize(res); err == nil {
			_, err = s.write(data, addr)
		}
		if err != nil {
			serverErrorLog.Println(err)
		}

	}
	wg.Done() // notify main goroutine about completion
}

// deserialize deserializes a sequence of bytes into Request structure.
func deserialize(data []byte, addr *net.UDPAddr) (*Request, error) {

	// validate input

	len := len(data)
	if len == 0 {
		return nil, errors.New("cannot deserialize empty data")
	}

	if addr == nil {
		return nil, errors.New("unknown client address")
	}

	// TODO: redo using request interface and multiple implementations

	var inq Inquiry
	payload := []byte{}

	switch inq = Inquiry(data[0]); inq {
	case Dnop:
		if len > 1 {
			// we have to copy the payload since the provided
			// slice is reusable read buffer
			payload = make([]byte, len-1)
			copy(payload, data[1:])
		}
	case Dping:
		// nothing to do here
	default:
		return nil, errors.New("unsupported request type")
	}

	return &Request{inq, addr, payload}, nil
}

// serialize serializes given Response structure into a sequence of bytes.
func serialize(res *Response) ([]byte, *net.UDPAddr, error) {

	// validate input

	if res == nil {
		return []byte{}, nil, errors.New("cannot serialize nil response")
	}

	// serialize response

	switch res.inq {
	case Dping:
		return []byte{byte(Dping)}, res.addr, nil
	default:
		// please note, the response for NOP inquiry is not expected as well
		return []byte{}, res.addr, errors.New("unsupported response type")
	}

}
