package dhcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/debug"
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
			s.cfg.logger.Debugf("serving request from %s", addr)
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
	return socket.ListenDHCP(s.cfg.logger, s.cfg.IP, &s.cfg.Interface)
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
			s.cfg.logger.Infof("Caught panic while serving a DHCP request from %s", addr.String())
			s.cfg.logger.Infof("\t%v", x)
			s.cfg.logger.Infof(string(debug.Stack()))
		}
	}()

	err := s.serveDHCPv4(c, payload, addr)
	if err != nil {
		s.cfg.logger.Warnf("failed to serve request from %s: %s", addr.String(), err.Error())
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

	resp.ServerIPAddr = cfg.IP

	// If the request message has the server identifier option set we must check
	// if it matches our server IP and drop the request entirely if not
	reqID := msg.ServerIdentifier()
	if reqID != nil && !reqID.IsUnspecified() && reqID.String() != cfg.IP.String() {
		s.cfg.logger.Debugf("ignoring packet with incorrect server ID %q from %s", reqID, msg.ClientHWAddr)
		return nil
	}
	// make sure to add the server identifier option to all DHCP messages
	// as per RFC2131
	resp.UpdateOption(dhcpv4.OptServerIdentifier(cfg.IP))

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

	cfg.logger.Debugf("-> %s from %s (%s)", msg.MessageType(), addr, msg.HostName())

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

	// Some clients require a directed unicast with correct destination IP (the same as in resp.YourIPAddr) and source MAC and IP.
	// Android for example ignores a DHCPOFFER that originates from 255.255.255.255 (ff:ff:ff:ff:ff:ff) rather than the specific
	// interface IP and hardware address.
	addr = tryMakeDirectedUnicastAddr(addr, cfg, resp)

	cfg.logger.Debugf("<- %s to %s (%s)", resp.MessageType(), addr, msg.HostName())

	response := resp.ToBytes()
	_, err = c.WriteTo(response, addr)
	return err
}

// tryMakeDirectiryUnicastAddres checks if addr is a *socket.Addr and updates the Local and Remote address pair (IP + MAC) to
// be as specific as possible by replacing an unspecified/broadcast source with the interface IP and MAC and an unspecified
// destination with the to-be-leased IP address from resp.YourIPAddr.
func tryMakeDirectedUnicastAddr(addr net.Addr, cfg *Config, resp *dhcpv4.DHCPv4) net.Addr {
	if a, ok := addr.(*socket.Addr); ok {
		if a.Local.IP.IsUnspecified() || a.Local.IP.String() == "255.255.255.255" {
			cfg.logger.Debugf("setting sender from %s (%s) to %s (%s)", a.Local.IP, a.Local.MAC, cfg.IP, cfg.Interface.HardwareAddr)
			a.Local.MAC = cfg.Interface.HardwareAddr
			a.Local.IP = cfg.IP
		}

		if a.RawAddr.IP.IsUnspecified() && resp.YourIPAddr != nil && !resp.YourIPAddr.IsUnspecified() {
			a.RawAddr.IP = resp.YourIPAddr
		}
	}

	return addr
}

// Compile-Time check
var _ caddy.Server = &Server{}
