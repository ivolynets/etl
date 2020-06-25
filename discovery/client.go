package discovery

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	_net "github.com/ivolynets/etl/internal/net"
)

// TODO: move logging capabilities to separate package

// Info level logger which outputs messages to the system standard output.
var clientInfoLog *log.Logger = log.New(os.Stdout, "discovery_client: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)

// Error level logger which outputs messages to the system error output.
var clientErrorLog *log.Logger = log.New(os.Stderr, "discovery_client: ", log.LstdFlags|log.Lmicroseconds|log.Lmsgprefix)

var broadcast = "255.255.255.255" // default broadcast IP address

// Client pings other nodes in the grid in order to get their current status.
// It sends UDP broadcast message and waiting for reply from those nodes.
type Client struct {
	conn  _net.UDPConn // UDP connection
	baddr *net.UDPAddr // broadcast address
}

// NewClient initializes a new discovery client and returns a pointer to it.
// The new client will be using a given UDP port for communication with
// discovery servers.
//
// Please note, the client does not allow to use either system or dynamic port
// numbers, only the range of port numbers from 1024 through 49151 are allowed
// for use.
func NewClient(port int) (*Client, error) {

	// validate input

	if port < 1024 || port > 49151 {
		return nil, fmt.Errorf("the use of system or dynamic ports is prohibited")
	}

	// initialize client

	var baddr *net.UDPAddr // broadcast address
	var laddr *net.UDPAddr // listening address
	var conn _net.UDPConn
	var err error

	if baddr, err = _net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", broadcast, port)); err != nil {
		return nil, err
	}

	if laddr, err = _net.ResolveUDPAddr("udp", ":0"); err != nil {
		return nil, err
	}

	if conn, err = _net.ListenUDP("udp", laddr); err != nil {
		clientErrorLog.Println(err)
		return nil, err
	}

	return &Client{conn, baddr}, nil
}

// Nop sends a broadcast message to discovery servers. The maximum length of
// message payload is 254 bytes. It also may be empty.
//
// This method may be used for testing purposes or deprectaed later.
func (c *Client) Nop(payload []byte) error {

	// The simple NOP message structure in the basic implementation is
	// following:
	//
	//		+---------+----------------+
	//		| 1 byte  | message type   |
	//		|---------|----------------|
	//		| n bytes | payload        |
	//		| ...     | ...            |
	//		+---------+----------------+
	//
	// The payload is simply a string with maximum length of 254 bytes (the
	// server read buffer is currently of fixed size of 256 bytes and first two
	// bytes are allocated for the message type and size).

	// validate input

	if len(payload) > 254 {
		return fmt.Errorf("payload cannot be longer than 254 bytes")
	}

	// please note, we are not handling errors when writing to buffer because
	// documentation says errors are always nil

	// add message type
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(Dnop))

	// add a payload
	buf.Write(payload)

	// send a message
	if err := c.send(buf.Bytes()); err != nil {
		clientErrorLog.Println(err)
		return err
	}

	return nil
}

// Ping sends a broadcast message to discovery servers, collects replies and
// returns them.
func (c *Client) Ping() ([]*Response, error) {

	// The simple ping message structure in the basic implementation is
	// following:
	//
	//		+---------+--------------+
	//		| 1 byte  | message type |
	//		+---------+--------------+
	//
	// There is no payload in ping message.

	// send a message

	var err error

	if err = c.send([]byte{byte(Dping)}); err != nil {
		clientErrorLog.Println(err)
		return nil, err
	}

	return c.receive(), nil
}

// Close properly closes the discovery client.
func (c *Client) Close() error {
	return c.conn.Close()
}

// send sends a datagram to the broadcast address.
func (c *Client) send(data []byte) error {

	// validate data

	l := len(data)
	if l <= 0 {
		return fmt.Errorf("cannot send empty data")
	}

	// send data

	if _, err := c.conn.WriteToUDP(data, c.baddr); err != nil {
		// TODO: write until bytes are written
		// TODO: retry if write failed
		return err
	}

	inq := Inquiry(data[0])
	clientInfoLog.Printf("sent_bytes = %d, type = %s, addr = %s\n", l, ops[inq], c.baddr)
	return nil
}

// receive collects and returns replies from other nodes. It is waiting for
// replies until timeout and then gives up returning what was collected so far.
// Currently timeout is hardcoded to 5 seconds.
//
// Please note that this method does not return error beacause in case of error
// it simply retries until either success or timeout.
func (c *Client) receive() []*Response {

	var res []*Response
	var addr *net.UDPAddr
	var n int
	var err error

	buf := make([]byte, 256) // TODO: revise buffer size or make it configurable

	// we are waiting for all replies for 5 seconds and then give up

	dl := time.Now().Add(5 * time.Second)
	for time.Now().Before(dl) {

		if err = c.conn.SetReadDeadline(time.Now().Add(time.Second)); err == nil {
			n, addr, err = c.conn.ReadFromUDP(buf)
		}

		if err != nil {
			// ignore timeout error
			if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
				clientErrorLog.Println(err)
				continue
			}
		}

		// please note that according to documentation even though timeout
		// occurred the portion of bytes still might have been read

		clientInfoLog.Printf("read_bytes = %d\n", n)
		if n > 0 {
			inq := Inquiry(buf[0])
			res = append(res, &Response{inq, addr, []byte{}})
			clientInfoLog.Printf("addr = %s, type = %s, data = %s\n", addr, ops[inq], "")
		}

	}

	return res
}
