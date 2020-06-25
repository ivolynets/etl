package discovery

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	_net "github.com/ivolynets/etl/internal/net"
)

/*----------------------------------------------------------------------------*/

// mockUDPConn is mocked UDP connection
type mockUDPConn struct {
	setReadDeadline func(t time.Time) error
	readFromUDP     func(b []byte) (int, *net.UDPAddr, error)
	writeToUDP      func(b []byte, addr *net.UDPAddr) (int, error)
	close           func() error
}

// SetReadDeadline is mocked UDPConn.SetReadDeadline() method.
func (c *mockUDPConn) SetReadDeadline(t time.Time) error {
	return c.setReadDeadline(t)
}

// ReadFromUDP is mocked UDPConn.ReadFromUDP() method.
func (c *mockUDPConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	return c.readFromUDP(b)
}

// WriteToUDP is mocked UDPConn.WriteToUDP() method.
func (c *mockUDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	return c.writeToUDP(b, addr)
}

// Close is mocked UDPConn.Close() method.
func (c *mockUDPConn) Close() error {
	return c.close()
}

/*----------------------------------------------------------------------------*/

// predefined ResolveUDPAddr() function mock implementation that returns with
// success
var okResolveUDPAddr = func(network, address string) (*net.UDPAddr, error) {
	return udpAddr, nil
}

// predefined ResolveUDPAddr() function mock implementation that fails
var errResolveUDPAddr = func(network, address string) (*net.UDPAddr, error) {
	return nil, errors.New("could not resolve address")
}

// predefined ListenUDP() function mock implementation that returns with
// success
var okListenUDP = func(network string, laddr *net.UDPAddr) (_net.UDPConn, error) {
	conn := udpConn
	conn.close = func() error {
		return nil
	}
	return conn, nil
}

// predefined ListenUDP() function mock implementation that fails
var errListenUDP = func(network string, laddr *net.UDPAddr) (_net.UDPConn, error) {
	return nil, errors.New("could not create UDP connection")
}

// predefined UDPConn.SetReadDeadline() method mock implementation that returns
// with success
var okSetReadDeadline = func(t time.Time) error { return nil }

// predefined UDPConn.SetReadDeadline() method mock implementation that fails
var errSetReadDeadline = func(t time.Time) error { return errors.New("could not set read deadline") }

// predefined UDPConn.ReadFromUDP() method mock implementation that returns
// with success
var okReadFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
	b[0] = 1
	return 1, udpAddr, nil
}

// predefined UDPConn.ReadFromUDP() method mock implementation that is timing
// out
var toReadFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
	return 0, nil, &net.OpError{Op: "read", Net: "", Source: nil, Addr: nil, Err: TimeoutError("read timeout")}
}

// predefined UDPConn.ReadFromUDP() method mock implementation that fails
var errReadFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
	return 0, nil, &net.OpError{Op: "read", Net: "", Source: nil, Addr: nil, Err: net.UnknownNetworkError("connection error")}
}

// predefined UDPConn.WriteToUDP() method mock implementation that returns with
// success
var okWriteToUDP = func(b []byte, addr *net.UDPAddr) (int, error) {
	return 1, nil
}

// predefined UDPConn.WriteToUDP() method mock implementation that fails
var errWriteToUDP = func(b []byte, addr *net.UDPAddr) (int, error) {
	return 0, &net.OpError{Op: "read", Net: "", Source: nil, Addr: nil, Err: net.UnknownNetworkError("connection error")}
}

// predefined UDPConn.Close() method mock implementation that returns with
// success
var okClose = func() error {
	return nil
}

// predefined UDPConn.Close() method mock implementation that fails
var errClose = func() error {
	return errors.New("unknown error")
}

/*----------------------------------------------------------------------------*/

// TimeoutError is a test network timeout error.
type TimeoutError string

func (e TimeoutError) Error() string   { return string(e) }
func (e TimeoutError) Timeout() bool   { return true }
func (e TimeoutError) Temporary() bool { return false }

/*----------------------------------------------------------------------------*/

// test UDP address
var udpAddr, _ = net.ResolveUDPAddr("udp", "localhost:0")

// test UDP connection
var udpConn = &mockUDPConn{okSetReadDeadline, okReadFromUDP, okWriteToUDP, okClose}

/*----------------------------------------------------------------------------*/

// TestPing performs a basic test in order to check how discovery client and
// server interact with each other. The client sends a ping request and then
// waiting for reply from the server. Then test verifies output logs of both
// client and server if they contain control messages. It also ensures error
// logs are empty.
func TestPing(t *testing.T) {

	var sstdout bytes.Buffer
	var sstderr bytes.Buffer
	var cstdout bytes.Buffer
	var cstderr bytes.Buffer

	serverInfoLog.SetOutput(&sstdout)
	serverErrorLog.SetOutput(&sstderr)
	clientInfoLog.SetOutput(&cstdout)
	clientErrorLog.SetOutput(&cstderr)

	server, _ := StartServer(9999)

	// wait for the server to be started
	if time.Sleep(time.Second); t.Failed() {
		t.FailNow()
	}

	var client *Client
	var err error

	if client, err = NewClient(9999); err != nil {
		t.Fatal(err)
	}

	client.Ping()
	server.Stop()

	// check control messages

	assertOutput(cstdout, "discovery_client: sent_bytes = 1, type = PING", t)
	assertOutput(sstdout, "discovery_server: request = PING", t)
	assertOutput(sstdout, "discovery_server: response = PING", t)
	assertOutput(cstdout, ", type = PING,", t)

	// check error logs

	if cstderr.Len() > 0 {
		t.Errorf("client log contains errors:\n%s", cstderr.Bytes())
	}

	if sstderr.Len() > 0 {
		t.Errorf("server log contains errors:\n%s", sstderr.Bytes())
	}

}

// assertOutput checks the output buffer if it contains an expected line. It
// fails the test if the line was not found.
func assertOutput(out bytes.Buffer, exp string, t *testing.T) {
	buf := bufio.NewReader(&out)
	for l, _, err := buf.ReadLine(); err != io.EOF; l, _, err = buf.ReadLine() {
		if strings.Contains(string(l), exp) {
			return
		}
	}
	t.Errorf("string '%s' has not been found in the output buffer", exp)
}
