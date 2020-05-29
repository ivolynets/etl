package discovery

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {

	var stdout bytes.Buffer
	infoLog.SetOutput(&stdout)

	var errout bytes.Buffer
	errorLog.SetOutput(&errout)

	server := NewServer()

	go func() error {
		return server.Start(":9999")
	}()

	addr, err := net.ResolveUDPAddr("udp", "255.255.255.255:9999")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Second)

	conn, err := net.DialUDP("udp", nil, addr)
	defer conn.Close()

	if err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Write([]byte("Hello")); err == nil {
		time.Sleep(10 * time.Second)
	} else {
		t.Fatal(err)
	}

	server.Stop()

	buf := bufio.NewReader(&stdout)
	ok := false
	for {
		if line, _, err := buf.ReadLine(); err == nil {
			if ok = strings.Contains(string(line), "data = Hello"); ok {
				break
			}
		} else {
			if err == io.EOF {
				break
			}
			fmt.Println(err)
		}
	}

	if !ok {
		t.Fatal("data has not been received by the server")
	}

}
