package dhcpserver

import (
	"net"
	"sync"
	"github.com/caddyserver/caddy"
)

// Server represents an instance of a server which
// serves DHCP clients on a particular interface.
// Each Server may serve multiple IP subnets on the
// same interface picking the first defined one as
// the default. The other subnets may be either queried
// directly by requesting an IP address located there
// or by the use of DHCP relay agents
type Server struct {
	dhcpWg sync.WaitGroup
}

// NewServer returns a new DHCPv4 server that compiles all plugins in to it
func NewServer(cfg []*Config) (*Server, error) {
	s := &Server{}
	
	s.dhcpWg.Add(1)

	return s, nil
}

// Serve is a NO-OP as TCP is not supported by dhcpserver. It
// implements the caddy.TCPServer interface
func (s *Server) Serve(l net.Listener) error {
	return nil
}

// ServePacket starts the server with an existing PacketConn. It blocks until
// the server stops. This implements the caddy.UDPServer interface
func (s *Server) ServePacket(c net.PacketConn) error {
	return nil	
}

// Listen does nothing as TCP is not supported. It implements the
// caddy.TCPServer interface
func (s *Server) Listen() (net.Listener, error) {
	return nil, nil
}

// ListenPacket starts listening for DHCP request messages via UDP/Raw sockets
// This implements the caddy.UDPServer interface
func (s *Server) ListenPacket() (net.PacketConn, error) {
	return nil, nil
}

// Compile-Time check
var _ caddy.Server = &Server{}