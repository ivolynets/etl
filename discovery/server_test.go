package discovery

import (
	"bytes"
	"container/list"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	_net "github.com/ivolynets/etl/internal/net"
)

// Tests StartServer() function how it behaves in the follofing scenarios:
//   - invalid port number provided.
//   - error occurred during the address resolution.
//   - error occurred while creating a UDP connection.
//   - server is successfully started.
func TestStartServer(t *testing.T) {

	type args struct {
		port int
	}

	tests := []struct {
		name           string
		resolveUDPAddr func(network, address string) (*net.UDPAddr, error)
		listenUDP      func(network string, laddr *net.UDPAddr) (_net.UDPConn, error)
		args           args
		want           bool
		wantErr        bool
	}{
		{"invalid port number", okResolveUDPAddr, okListenUDP, args{80}, false, true},
		{"address resolution error", errResolveUDPAddr, okListenUDP, args{9999}, false, true},
		{"connection error", okResolveUDPAddr, errListenUDP, args{9999}, false, true},
		{"successful start", okResolveUDPAddr, okListenUDP, args{9999}, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tmp0 := _net.ResolveUDPAddr
			tmp1 := _net.ListenUDP
			_net.ResolveUDPAddr = tt.resolveUDPAddr
			_net.ListenUDP = tt.listenUDP

			got, err := StartServer(tt.args.port)
			if got != nil {
				got.Stop()
			}

			_net.ResolveUDPAddr = tmp0
			_net.ListenUDP = tmp1

			if (err != nil) != tt.wantErr {
				t.Errorf("StartServer() error = %v, wantErr %v", err != nil, tt.wantErr)
				return
			}
			if (got != nil) != tt.want {
				t.Errorf("StartServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests Server.read() method how it behaves in the follofing scenarios:
//   - input buffer for reading data is nil.
//   - input buffer for reading data has 0 size.
//   - when setting of the read timeout fails. In this case it is expected the
//     method will return 0 bytes read, nil address and corresponding error.
//   - when read operation from the UDP connection timed out. This is
//     considered a valid case and no error should be returned. Instead it
//     should simply return 0 bytes read. Returned address may have any value.
//   - when the read from the UDP connection fails due to some issue other than
//     timeout. In this case method should return an error received from the
//     connection. All other return parameters may have any value.
//   - when the read from the UDP connection was successfull. In this case it
//     should return the correct number of bytes read, remote address and no
//     error.
func TestServer_read(t *testing.T) {

	server := &Server{server{conn: &mockUDPConn{}}}

	type args struct {
		buf []byte
	}

	type rets struct {
		buf   []byte
		bytes int
		addr  *net.UDPAddr
		err   bool
	}

	tests := []struct {
		name            string
		server          *Server
		setReadDeadline func(t time.Time) error
		readFromUDP     func(b []byte) (int, *net.UDPAddr, error)
		args            args
		rets            rets
	}{
		{"nil input buffer", server, okSetReadDeadline, okReadFromUDP, args{nil}, rets{nil, 0, nil, true}},
		{"empty input buffer", server, okSetReadDeadline, okReadFromUDP, args{[]byte{}}, rets{[]byte{}, 0, nil, true}},
		{"set timeout error", server, errSetReadDeadline, okReadFromUDP,
			args{[]byte{0}}, rets{[]byte{0}, 0, nil, true}},
		{"read timeout", server, okSetReadDeadline, toReadFromUDP, args{[]byte{0}}, rets{[]byte{0}, 0, nil, false}},
		{"read error", server, okSetReadDeadline, errReadFromUDP, args{[]byte{0}}, rets{[]byte{0}, 0, nil, true}},
		{"successful read", server, okSetReadDeadline, okReadFromUDP, args{[]byte{0}}, rets{[]byte{1}, 1, udpAddr, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tt.server.conn.(*mockUDPConn).setReadDeadline = tt.setReadDeadline
			tt.server.conn.(*mockUDPConn).readFromUDP = tt.readFromUDP

			gotBytes, gotAddr, gotErr := tt.server.read(tt.args.buf)

			if !bytes.Equal(tt.args.buf, tt.rets.buf) {
				t.Errorf("Server.read() gotBuf = %v, want %v", tt.args.buf, tt.rets.buf)
				return
			}

			if (gotErr != nil) != tt.rets.err {
				t.Errorf("Server.read() gotErr = %v, want %v", gotErr, tt.rets.err)
			}
			if gotBytes != tt.rets.bytes {
				t.Errorf("Server.read() gotBytes = %v, want %v", gotBytes, tt.rets.bytes)
			}
			if !reflect.DeepEqual(gotAddr, tt.rets.addr) {
				t.Errorf("Server.read() gotAddr = %v, want %v", gotAddr, tt.rets.addr)
			}

		})
	}
}

// Tests Server.write() method how it behaves in the follofing scenarios:
//   - input data slice is nil.
//   - input data is empty.
//   - destination address is nil.
//   - when the write to the UDP connection fails. In this case method should
//     return an error received from the connection. All other return
//     parameters may have any value.
//   - when the write to the UDP connection was successfull. In this case it
//     should return the correct number of bytes written and no error.
func TestServer_write(t *testing.T) {

	server := &Server{server{conn: &mockUDPConn{}}}

	type args struct {
		data []byte
		addr *net.UDPAddr
	}

	type rets struct {
		bytes int
		err   bool
	}

	tests := []struct {
		name       string
		server     *Server
		writeToUDP func(b []byte, addr *net.UDPAddr) (int, error)
		args       args
		rets       rets
	}{
		{"nil input data", server, okWriteToUDP, args{nil, udpAddr}, rets{0, true}},
		{"empty input data", server, okWriteToUDP, args{[]byte{}, udpAddr}, rets{0, true}},
		{"nil destination address", server, nil, args{[]byte{0}, nil}, rets{0, true}},
		{"write error", server, errWriteToUDP, args{[]byte{0}, udpAddr}, rets{0, true}},
		{"successful write", server, okWriteToUDP, args{[]byte{0}, udpAddr}, rets{1, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tt.server.conn.(*mockUDPConn).writeToUDP = tt.writeToUDP
			gotBytes, gotErr := tt.server.write(tt.args.data, tt.args.addr)

			if (gotErr != nil) != tt.rets.err {
				t.Errorf("Server.write() gotErr = %v, want %v", gotErr, tt.rets.err)
			}
			if gotBytes != tt.rets.bytes {
				t.Errorf("Server.write() gotBytes = %v, want %v", gotBytes, tt.rets.bytes)
			}
		})
	}
}

// Tests run() method how it behaves in the follofing scenarios:
//   - read operation has timed out. In this case no request should be produced
//     to the internal buffer.
//   - read operation returned with the error. In this case the read cycle
//     should be simply skipped with corresponding error message written to the
//     log.
//   - valid inquiry is received. In this case a request should be produced to
//     the internal buffer so that worker(s) can handle it.
//   - invalid (unsupported) inquiry is received. In this case deserialization
//     should fail and the read cycle should be skipped with corresponding
//     error message written to the log.
func TestServer_run(t *testing.T) {

	// set up

	buf := make(chan *Request, 3)
	ctl := make(chan byte)
	conn := &mockUDPConn{}
	server := &Server{server{conn: conn, buf: buf, ctl: ctl}}

	type read struct {
		data []byte
		addr *net.UDPAddr
		err  error
	}

	tests := []struct {
		name string
		read read
		req  *Request
	}{
		{"read timeout", read{[]byte{}, nil, &net.OpError{Op: "read", Net: "", Source: nil, Addr: nil, Err: TimeoutError("read timeout")}}, nil},
		{"read error", read{[]byte{}, nil, &net.OpError{Op: "read", Net: "", Source: nil, Addr: nil, Err: net.UnknownNetworkError("connection error")}}, nil},
		{"valid inquiry", read{[]byte{1}, udpAddr, nil}, &Request{Dping, udpAddr, []byte{}}},
		{"invalid inquiry", read{[]byte{255}, udpAddr, nil}, nil},
	}

	i := -1
	conn.readFromUDP = func(b []byte) (int, *net.UDPAddr, error) {
		i++
		if i < len(tests) {
			n := copy(b, tests[i].read.data)
			return n, tests[i].read.addr, tests[i].read.err
		}
		server.Stop()
		return 0, nil, nil
	}
	conn.setReadDeadline = func(t time.Time) error { return nil }
	conn.close = func() error { return nil }

	// run and read inquiries

	var wg sync.WaitGroup
	go server.run(&wg)

	// validate results

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.req == nil {
				return
			}

			var req *Request
			var ok bool

			if req, ok = <-server.buf; !ok {
				t.Error("Server.run() request is expected but nothing received")
			}
			if req.inq != tt.req.inq {
				t.Errorf("Server.run() gotInq = %v, want %v", req.inq, tt.req.inq)
			}
			if !bytes.Equal(req.payload, tt.req.payload) {
				t.Errorf("Server.run() gotData = %v, want %v", req.payload, tt.req.payload)
			}
			if !reflect.DeepEqual(req.addr, tt.req.addr) {
				t.Errorf("Server.run() gotAddr = %v, want %v", req.addr, tt.req.addr)
			}
		})
	}

	if len(server.buf) > 0 {
		t.Errorf("Server.run() more %v request(s) available but nothing is expected", len(server.buf))
	}
}

// Tests handleRequests() method how it behaves in the follofing scenarios:
//   - NOP request is received. In this case no response and write to UDP is
//     expected.
//   - PING request is received. In this case response should be a PING message
//     as well.
func TestServer_handleRequests(t *testing.T) {

	// set up

	buf := make(chan *Request, 3)
	ctl := make(chan byte)
	conn := &mockUDPConn{}
	server := &Server{server{conn: conn, buf: buf, ctl: ctl}}

	type res struct {
		data []byte
		addr *net.UDPAddr
	}

	tests := []struct {
		name   string
		server *Server
		req    *Request
		res    *res
	}{
		{"NOP request", server, &Request{Dnop, udpAddr, []byte("Hello")}, nil},
		{"PING request", server, &Request{Dping, udpAddr, []byte{}}, &res{[]byte{1}, udpAddr}},
		{"unsupported request", server, &Request{255, udpAddr, []byte{}}, nil},
	}

	// send and handle requests

	writes := list.New()
	server.conn.(*mockUDPConn).writeToUDP = func(b []byte, addr *net.UDPAddr) (int, error) {
		writes.PushBack(&res{b, addr})
		return len(b), nil
	}

	for _, tt := range tests {
		buf <- tt.req
	}
	close(buf)

	var wg sync.WaitGroup
	wg.Add(1)
	server.handleRequests(&wg)

	// validate results

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.res == nil {
				return
			}

			write := writes.Front()

			if write == nil {
				t.Error("Server.handleRequests() write is expected but nothing was written")
			}
			if !bytes.Equal(write.Value.(*res).data, tt.res.data) {
				t.Errorf("Server.handleRequests() gotData = %v, want %v", write.Value.(*res).data, tt.res.data)
			}
			if !reflect.DeepEqual(write.Value.(*res).addr, tt.res.addr) {
				t.Errorf("Server.handleRequests() gotAddr = %v, want %v", write.Value.(*res).addr, tt.res.addr)
			}
			writes.Remove(write)
		})
	}

	if writes.Len() > 0 {
		t.Errorf("Server.handleRequests() there were %v additional unexpected writes", writes.Len())
	}

}

// Tests deserialize() function how it behaves in the follofing scenarios:
//   - input data slice is nil.
//   - input data is empty.
//   - source address is nil.
//   - the inquiry type is not supported by this function.
//   - when the NOP inquiry comes.
//   - when the NOP inquiry comes with corrupted data (the number of bytes
//     received is less than expected).
//   - when the NOP inquiry comes with additional bytes which are not part of
//     request.
//	 - when the PING inquiry comes.
//   - when the PING inquiry comes with additional bytes which are not part of
//     request.
func Test_deserialize(t *testing.T) {

	type args struct {
		data []byte
		addr *net.UDPAddr
	}

	type rets struct {
		req *Request
		err bool
	}

	tests := []struct {
		name string
		args args
		rets rets
	}{
		{"nil input data", args{nil, udpAddr}, rets{nil, true}},
		{"empty input data", args{[]byte{}, udpAddr}, rets{nil, true}},
		{"nil source address", args{[]byte{0}, nil}, rets{nil, true}},
		{"unsupported request type", args{[]byte{255}, udpAddr}, rets{nil, true}},
		{"NOP request", args{[]byte{0, 65}, udpAddr}, rets{&Request{Dnop, udpAddr, []byte("A")}, false}},
		{"NOP request has more bytes", args{[]byte{0, 65, 66, 67}, udpAddr}, rets{&Request{Dnop, udpAddr, []byte("ABC")}, false}},
		{"ping request", args{[]byte{1}, udpAddr}, rets{&Request{Dping, udpAddr, []byte{}}, false}},
		{"ping request has more bytes", args{[]byte{1, 65, 66}, udpAddr}, rets{&Request{Dping, udpAddr, []byte{}}, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotReq, gotErr := deserialize(tt.args.data, tt.args.addr)

			if (gotErr != nil) != tt.rets.err {
				t.Errorf("deserialize() gotErr = %v, want %v", gotErr, tt.rets.err)
			}
			if !reflect.DeepEqual(gotReq, tt.rets.req) {
				t.Errorf("deserialize() gotReq = %v, want %v", *gotReq, *tt.rets.req)
			}
		})
	}
}

// Tests serialize() function how it behaves in the follofing scenarios:
//   - input response structure is nil.
//   - unsupported type of response is provided.
//   - explicit check of NOP response because it is intentionally not supported.
//   - serialization of normal PING response.
//   - serialization of PING response with nil destination address. In this
//     case the function is not expected to fail and simply return nil address.
//     It is not responsibility of serializer to decide what to do if the
//     destiation address is not provided.
//   - serialization of PING response with payload provided. In this case the
//     payload should be simply ignored.
func Test_serialize(t *testing.T) {

	type args struct {
		res *Response
	}

	type rets struct {
		data []byte
		addr *net.UDPAddr
		err  bool
	}

	tests := []struct {
		name string
		args args
		rets rets
	}{
		{"nil input response", args{nil}, rets{[]byte{}, nil, true}},
		{"unsupported response type", args{&Response{255, udpAddr, []byte{}}}, rets{[]byte{}, udpAddr, true}},
		{"unsupported NOP response", args{&Response{0, udpAddr, []byte{}}}, rets{[]byte{}, udpAddr, true}},
		{"ping response", args{&Response{1, udpAddr, []byte{}}}, rets{[]byte{1}, udpAddr, false}},
		{"ping response with no address", args{&Response{1, nil, []byte{}}}, rets{[]byte{1}, nil, false}},
		{"ping response with payload provided", args{&Response{1, udpAddr, []byte("A")}}, rets{[]byte{1}, udpAddr, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gotData, gotAddr, gotErr := serialize(tt.args.res)

			if (gotErr != nil) != tt.rets.err {
				t.Errorf("serialize() gotErr = %v, want %v", gotErr, tt.rets.err)
			}
			if !bytes.Equal(gotData, tt.rets.data) {
				t.Errorf("serialize() gotData = %v, want %v", gotData, tt.rets.data)
			}
			if !reflect.DeepEqual(gotAddr, tt.rets.addr) {
				t.Errorf("serialize() gotAddr = %v, want %v", gotAddr, tt.rets.addr)
			}
		})
	}
}
