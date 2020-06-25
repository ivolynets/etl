package discovery

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	_net "github.com/ivolynets/etl/internal/net"
)

// Tests NewClient() function how it behaves in the follofing scenarios:
//   - invalid port number provided.
//   - error occurred during the address resolution.
//   - client is successfully created.
func TestNewClient(t *testing.T) {

	type args struct {
		port int
	}

	tests := []struct {
		name         string
		resolveBAddr func(network, address string) (*net.UDPAddr, error)
		resolveLAddr func(network, address string) (*net.UDPAddr, error)
		listenUDP    func(network string, laddr *net.UDPAddr) (_net.UDPConn, error)
		args         args
		want         *Client
		wantErr      bool
	}{
		{"invalid port number", okResolveUDPAddr, okResolveUDPAddr, okListenUDP, args{80}, nil, true},
		{"broadcast address resolution error", errResolveUDPAddr, okResolveUDPAddr, okListenUDP, args{9999}, nil, true},
		{"listening address resolution error", okResolveUDPAddr, errResolveUDPAddr, okListenUDP, args{9999}, nil, true},
		{"listen UDP port error", okResolveUDPAddr, okResolveUDPAddr, errListenUDP, args{9999}, nil, true},
		{"successfully created", okResolveUDPAddr, okResolveUDPAddr, okListenUDP, args{9999}, &Client{udpConn, udpAddr}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tmp0 := _net.ResolveUDPAddr
			tmp1 := _net.ListenUDP

			_net.ResolveUDPAddr = func(network, addr string) (*net.UDPAddr, error) {
				switch addr {
				case fmt.Sprintf("%s:%d", broadcast, tt.args.port):
					return tt.resolveBAddr(network, addr)
				case ":0":
					return tt.resolveLAddr(network, addr)
				}
				t.Errorf("NewClient() unexpected call of ResolveUDPAddr(%v, %v)", network, addr)
				return net.ResolveUDPAddr(network, addr)
			}
			_net.ListenUDP = tt.listenUDP

			got, err := NewClient(tt.args.port)
			_net.ResolveUDPAddr = tmp0
			_net.ListenUDP = tmp1

			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err != nil, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests Client.send() method how it behaves in the follofing scenarios:
//   - nil data is given. This is invalid case because send method should not
//     be called if there is nothing to send.
//   - empty data is given. This is invalid case because send method should not
//     be called if there is nothing to send.
//   - error occurred when sending the datagram.
//   - the data was provided and a message was successfully sent.
func TestClient_send(t *testing.T) {

	conn := &mockUDPConn{}
	client := &Client{conn, udpAddr}

	type args struct {
		data []byte
	}

	tests := []struct {
		name       string
		client     *Client
		writeToUDP func(b []byte, addr *net.UDPAddr) (int, error)
		args       args
		wantErr    bool
	}{
		{"nil input data", client, okWriteToUDP, args{nil}, true},
		{"empty input data", client, okWriteToUDP, args{[]byte{}}, true},
		{"write error", client, errWriteToUDP, args{[]byte{0}}, true},
		{"successful write", client, okWriteToUDP, args{[]byte{0}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.client.conn.(*mockUDPConn).writeToUDP = tt.writeToUDP
			if err := tt.client.send(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("Client.send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Tests Client.receive() method how it behaves in the follofing scenarios:
//   - error occurred when setting up the read timeout. In this case method
//     should simply retry until either success or receive timeout (wait period).
//   - read operation timed out. In this case method should simply retry until
//     either success or receive timeout (wait period).
//   - successfully received a message.
func TestClient_receive(t *testing.T) {

	conn := &mockUDPConn{}
	client := &Client{conn, udpAddr}

	tests := []struct {
		name            string
		client          *Client
		setReadDeadline func(t time.Time) error
		readFromUDP     func(b []byte) (int, *net.UDPAddr, error)
		want            []*Response
	}{
		{"set timeout error", client, errSetReadDeadline, okReadFromUDP, nil},
		{"read timeout", client, okSetReadDeadline, toReadFromUDP, nil},
		{"read error", client, okSetReadDeadline, errReadFromUDP, nil},
		{"successful read", client, okSetReadDeadline, okReadFromUDP, []*Response{{Dping, udpAddr, []byte{}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			read := false

			tt.client.conn.(*mockUDPConn).setReadDeadline = func(t time.Time) error {
				err := tt.setReadDeadline(t)
				if err != nil {
					time.Sleep(time.Second)
				}
				return err
			}
			tt.client.conn.(*mockUDPConn).readFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
				if !read {
					read = true
					return tt.readFromUDP(b)
				}
				time.Sleep(time.Second)
				return 0, nil, nil
			}

			if got := tt.client.receive(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Client.receive() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests Client.Nop() method how it behaves in the follofing scenarios:
//   - oversized payload is given. The size of payload is limited by discovery
//     server, also it may cause a panic when writing to the internal byte
//     buffer. Method should return error if the payload is too big.
//   - nil payload is given. This is valid case since no payload is accepted.
//   - empty payload is given. This is valid case since no payload is accepted.
//   - error occurred when sending the message.
//   - the payload of a normal size is given and a message was successfully
//     sent.
func TestClient_Nop(t *testing.T) {

	conn := &mockUDPConn{}
	client := &Client{conn, udpAddr}

	type args struct {
		payload []byte
	}

	tests := []struct {
		name       string
		client     *Client
		writeToUDP func(b []byte, addr *net.UDPAddr) (int, error)
		args       args
		wantErr    bool
	}{
		{"oversized payload", client, okWriteToUDP, args{make([]byte, 256)}, true},
		{"nil payload", client, okWriteToUDP, args{nil}, false},
		{"empty payload", client, okWriteToUDP, args{[]byte{}}, false},
		{"write error", client, errWriteToUDP, args{[]byte{}}, true},
		{"successful write", client, okWriteToUDP, args{[]byte{0}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.client.conn.(*mockUDPConn).writeToUDP = tt.writeToUDP
			if err := tt.client.Nop(tt.args.payload); (err != nil) != tt.wantErr {
				t.Errorf("Client.Nop() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Tests Client.Ping() method how it behaves in the follofing scenarios:
//   - error occurred when sending the message.
//   - error occurred when setting up the read timeout. In this case method
//     should simply retry until either success or receive timeout (wait period).
//   - read operation timed out. In this case method should simply retry until
//     either success or receive timeout (wait period).
//   - successfully sent a message and received a reply.
func TestClient_Ping(t *testing.T) {

	conn := &mockUDPConn{}
	client := &Client{conn, udpAddr}

	tests := []struct {
		name            string
		client          *Client
		writeToUDP      func(b []byte, addr *net.UDPAddr) (int, error)
		setReadDeadline func(t time.Time) error
		readFromUDP     func(b []byte) (int, *net.UDPAddr, error)
		want            []*Response
		wantErr         bool
	}{
		{"write error", client, errWriteToUDP, okSetReadDeadline, okReadFromUDP, nil, true},
		{"set timeout error", client, okWriteToUDP, errSetReadDeadline, okReadFromUDP, nil, false},
		{"read timeout", client, okWriteToUDP, okSetReadDeadline, toReadFromUDP, nil, false},
		{"read error", client, okWriteToUDP, okSetReadDeadline, errReadFromUDP, nil, false},
		{"successful read", client, okWriteToUDP, okSetReadDeadline, okReadFromUDP, []*Response{{Dping, udpAddr, []byte{}}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			read := false

			tt.client.conn.(*mockUDPConn).writeToUDP = tt.writeToUDP
			tt.client.conn.(*mockUDPConn).setReadDeadline = func(t time.Time) error {
				err := tt.setReadDeadline(t)
				if err != nil {
					time.Sleep(time.Second)
				}
				return err
			}
			tt.client.conn.(*mockUDPConn).readFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
				if !read {
					read = true
					return tt.readFromUDP(b)
				}
				time.Sleep(time.Second)
				return 0, nil, nil
			}

			got, err := tt.client.Ping()

			if (err != nil) != tt.wantErr {
				t.Errorf("Client.Ping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Client.Ping() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests Client.Close() method how it behaves in the follofing scenarios:
//   - error occurred when closing the client underlying UDP connection. In
//     this case Close() method should return a corresponding error as well.
//   - client is successfully closed.
func TestClient_Close(t *testing.T) {

	conn := &mockUDPConn{}
	client := &Client{conn, udpAddr}

	tests := []struct {
		name    string
		client  *Client
		close   func() error
		wantErr bool
	}{
		{"close error", client, errClose, true},
		{"successfully closed", client, okClose, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.client.conn.(*mockUDPConn).close = tt.close
			if err := tt.client.Close(); (err != nil) != tt.wantErr {
				t.Errorf("Client.Close() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
