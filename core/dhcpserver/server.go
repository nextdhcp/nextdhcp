package dhcpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/socket"
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
	cfg    *Config
}

// NewServer returns a new DHCPv4 server that compiles all plugins in to it
func NewServer(cfg *Config) (*Server, error) {
	s := &Server{cfg: cfg}

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
	// TODO(ppacher): we may remove the following check
	_, ok := c.(*socket.DHCPConn)
	if !ok {
		return errors.New("expected socket.DHCPConn")
	}
	for {
		payload := make([]byte, 4096)
		byteLen, addr, err := c.ReadFrom(payload)

		if byteLen > 0 {
			log.Println("serving request from ", addr)
			s.dhcpWg.Add(1)
			go s.serveAndLogDHCPv4(c, payload[:byteLen], addr)
		}

		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Temporary() || opErr.Timeout() {
					continue
				}
			}

			return err
		}
	}
}

// Listen does nothing as TCP is not supported. It implements the
// caddy.TCPServer interface
func (s *Server) Listen() (net.Listener, error) {
	return nil, nil
}

// ListenPacket starts listening for DHCP request messages via UDP/Raw sockets
// This implements the caddy.UDPServer interface
func (s *Server) ListenPacket() (net.PacketConn, error) {
	return socket.ListenDHCP(s.cfg.IP, &s.cfg.Interface)
}

// OnStartupComplete is called when all serves of the same instance have
// been started. It implements the caddy.AfterStarup interface
func (s *Server) OnStartupComplete() {
	info := getStartupInfo([]*Config{s.cfg})
	if info != "" {
		// Print not Println because info contains a trailing new line
		fmt.Print(info)
	}
}

func (s *Server) serveAndLogDHCPv4(c net.PacketConn, payload []byte, addr net.Addr) {
	defer s.dhcpWg.Done()
	// In any case we must not panic while serving requests
	defer func() {
		if x := recover(); x != nil {
			log.Printf("Caught panic while serving a DHCP request from %s", addr.String())
			log.Println(x)
		}
	}()

	err := s.serveDHCPv4(c, payload, addr)
	if err != nil {
		log.Printf("failed to serve request from %s: %s", addr.String(), err.Error())
	}
}

func (s *Server) findSubnetConfig(gwIP net.IP) *Config {
	return s.cfg
}

func (s *Server) serveDHCPv4(c net.PacketConn, payload []byte, addr net.Addr) error {
	msg, err := dhcpv4.FromBytes(payload)
	if err != nil {
		return err
	}

	cfg := s.findSubnetConfig(msg.GatewayIPAddr)
	if cfg == nil {
		return errors.New("subnet not served")
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg)
	if err != nil {
		return err
	}

	switch msg.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		// Response message type for Request (either ACK or NAK) should be set
		// by plugins
		fallthrough

	default:
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNone))
	}

	ctx := context.Background()
	ctx = lease.WithDatabase(ctx, cfg.Database)
	ctx = WithPeer(ctx, addr)

	err = cfg.chain.ServeDHCP(ctx, msg, resp)
	if err != nil && err != ErrNoResponse {
		return err
	}

	if err == ErrNoResponse {
		return nil
	}

	response := resp.ToBytes()
	_, err = c.WriteTo(response, addr)
	return err
}

// Compile-Time check
var _ caddy.Server = &Server{}
