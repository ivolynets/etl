package net

import (
	"net"
	"time"
)

// UDPConn is an interface that represents UDP connection. This interface is
// introduced as an abstraction layer between application and standard Go
// implementation in order to be able to mock for testing purposes.
type UDPConn interface {
	SetReadDeadline(t time.Time) error
	ReadFromUDP(b []byte) (int, *net.UDPAddr, error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (int, error)
	Close() error
}

// ResolveUDPAddr is a proxy for the standard net.ResolveUDPAddr() function
// which allows extension and mocking of its implementation.
var ResolveUDPAddr = func(network, address string) (*net.UDPAddr, error) {
	return net.ResolveUDPAddr(network, address)
}

// ListenUDP is a proxy for the standard net.ListenUDP() function which allows
// to extension and mocking of its implementation.
var ListenUDP = func(network string, laddr *net.UDPAddr) (UDPConn, error) {
	return net.ListenUDP(network, laddr)
}
