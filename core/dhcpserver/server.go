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
	"github.com/nextdhcp/nextdhcp/core/log"
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
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNone))

	cfg.logger.Debugf("-> %s from %s (%s)", msg.MessageType(), addr, msg.HostName())

	ctx := context.Background()
	ctx = lease.WithDatabase(ctx, cfg.Database)
	ctx = WithPeer(ctx, addr)
	ctx = log.AddRequestFields(ctx, msg)
	err = cfg.chain.ServeDHCP(ctx, msg, resp)
	if err != nil && err != ErrNoResponse {
		return err
	}

	if err == ErrNoResponse {
		return nil
	}

	addr = updateConnectionAddresses(ctx, addr, cfg, msg, resp)

	cfg.logger.Debugf("<- %s to %s (%s)", resp.MessageType(), addr, msg.HostName())

	response := resp.ToBytes()
	_, err = c.WriteTo(response, addr)
	return err
}

// updateConnectionAddresses tries to get the correct source and destination connection tuples (IP + MAC)
// as defined by RCP
func updateConnectionAddresses(ctx context.Context, addr net.Addr, cfg *Config, req, resp *dhcpv4.DHCPv4) net.Addr {
	l := log.With(ctx, cfg.logger)

	// From RFC (https://tools.ietf.org/html/rfc2131):
	//
	// (relays not yet supported)
	//
	// [If the 'giaddr' field in a DHCP message from a client is non-zero,
	// the server sends any return messages to the 'DHCP server' port on the
	// BOOTP relay agent whose address appears in 'giaddr'.] If the 'giaddr'
	// field is zero and the 'ciaddr' field is nonzero, then the server
	// unicasts DHCPOFFER and DHCPACK messages to the address in 'ciaddr'.
	// If 'giaddr' is zero and 'ciaddr' is zero, and the broadcast bit is
	// set, then the server broadcasts DHCPOFFER and DHCPACK messages to
	// 0xffffffff. If the broadcast bit is not set and 'giaddr' is zero and
	// 'ciaddr' is zero, then the server unicasts DHCPOFFER and DHCPACK
	// messages to the client's hardware address and 'yiaddr' address.  In
	// all cases, when 'giaddr' is zero, the server broadcasts any DHCPNAK
	// messages to 0xffffffff.
	if a, ok := addr.(*socket.Addr); ok {
		// if we known our local IP and MAC address we'll use that for sending
		if a.Local.IP.IsUnspecified() || a.Local.IP.String() == "255.255.255.255" {
			a.Local.MAC = cfg.Interface.HardwareAddr
			a.Local.IP = cfg.IP
		}

		if req.GatewayIPAddr == nil || req.GatewayIPAddr.IsUnspecified() {
			if req.ClientIPAddr != nil && !req.ClientIPAddr.IsUnspecified() {
				if Offer(resp) || Ack(resp) {
					a.RawAddr.IP = req.ClientIPAddr
					l.Debugf("unicasting to ciaddr %s (%s)", req.ClientIPAddr, a.RawAddr.MAC)
					return a
				}
			}

			if req.ClientIPAddr == nil || req.ClientIPAddr.IsUnspecified() {
				if req.IsBroadcast() {
					a.RawAddr.IP = net.IP{0xff, 0xff, 0xff, 0xff}
					l.Debugf("broadcasting to %s (%s) (broadcast bit set)", a.RawAddr.IP, a.RawAddr.MAC)
				} else {
					a.RawAddr.IP = resp.YourIPAddr
					l.Debugf("unicasting to yiaddr %s (%s)", a.RawAddr.IP, a.RawAddr.MAC)
				}

				return addr
			}

			if Nak(resp) {
				a.RawAddr.IP = net.IP{0xff, 0xff, 0xff, 0xff}
				l.Debugf("broadcasting to %s (%s) (NAK)", a.RawAddr.IP, a.RawAddr.MAC)
				return addr
			}

			l.Debugf("sending (unmodified) response to %s (%s)", a.RawAddr.IP, a.RawAddr.MAC)
		} else {
			l.Warnf("DHCP relay agents are not yet supported. giaddr=%s", req.GatewayIPAddr)
			return addr
		}
	}

	return addr
}

// Compile-Time check
var _ caddy.Server = &Server{}
