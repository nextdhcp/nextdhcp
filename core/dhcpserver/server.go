package dhcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/caddyserver/caddy/v2"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/socket"
	"github.com/nextdhcp/nextdhcp/core/utils/iface"
	"github.com/nextdhcp/nextdhcp/plugin"
	"go.uber.org/zap"
)

// listenDHCP opens the connection a server receives requests on and sends
// responses through. Tests replace it so they don't need raw sockets.
var listenDHCP = func(l log.Logger, ip net.IP, ifc *net.Interface) (net.PacketConn, error) {
	return socket.ListenDHCP(l, ip, ifc)
}

// Server serves DHCP clients on one subnet. Each server block of the Dhcpfile
// is adapted to one Server.
type Server struct {
	// Subnet is the subnet to serve, and is required. It is either an IPv4
	// address in CIDR notation, where the address is the server's own address
	// on the subnet (for example 192.168.0.1/24), or the name of a network
	// interface that has exactly one IPv4 address.
	Subnet string `json:"subnet,omitempty"`

	// Interface is the name of the network interface to serve the subnet on.
	// It defaults to the interface that has the server's address assigned.
	Interface string `json:"interface,omitempty"`

	// DatabaseConfig configures the lease database. It defaults to a bolt
	// database in the file "<server address>.db" in the working directory.
	DatabaseConfig *DatabaseConfig `json:"database,omitempty"`

	// HandlersRaw is the handler chain, in the order requests are passed
	// through it. Each handler decides whether to call the next one.
	HandlersRaw []json.RawMessage `json:"handle,omitempty" caddy:"namespace=dhcp.handlers inline_key=handler"`

	// IP is the server's own address on the subnet. It is set when the server
	// is provisioned.
	IP net.IP `json:"-"`

	// Network is the subnet. It is set when the server is provisioned.
	Network net.IPNet `json:"-"`

	// Database is the lease database that is queried for new leases and
	// reservations. It is opened when the server is provisioned.
	Database lease.Database `json:"-"`

	iface  net.Interface        // the interface the subnet is served on
	store  storage.LeaseStorage // the storage of the lease database
	chain  plugin.Handler       // the first handler of the chain
	logger *zap.SugaredLogger
	failed chan<- error // receives the error that stops the server from serving

	conn     net.PacketConn
	stopping atomic.Bool    // set by stop, after which reading from conn fails
	serveWg  sync.WaitGroup // the read loop
	dhcpWg   sync.WaitGroup // requests being served
}

// Provision resolves the subnet and the interface, opens the lease database
// and loads the handler chain. Handlers can get the server from the context
// they are provisioned with, using ServerFromContext.
func (s *Server) Provision(ctx caddy.Context) error {
	s.logger = ctx.Logger().Sugar()

	ip, ipNet, err := iface.ByNameOrCIDR(s.Subnet)
	if err != nil {
		return fmt.Errorf("failed to get subnet configuration for %s: %s", s.Subnet, err.Error())
	}
	s.IP = ip
	s.Network = *ipNet

	if s.Interface != "" {
		ifc, err := net.InterfaceByName(s.Interface)
		if err != nil {
			return fmt.Errorf("failed to find interface with name %s: %s", s.Interface, err.Error())
		}
		s.iface = *ifc
	}

	if !findInterface(s) {
		return fmt.Errorf("failed to find interface for subnet %s", s.Network.String())
	}

	if err := s.openDatabase(); err != nil {
		return fmt.Errorf("failed to open database for subnet %s: %s", s.Network.String(), err.Error())
	}

	mods, err := WithServer(ctx, s).LoadModule(s, "HandlersRaw")
	if err != nil {
		return fmt.Errorf("failed to load handlers for subnet %s: %s", s.Network.String(), err.Error())
	}

	var handlers []plugin.Middleware
	for i, m := range mods.([]any) {
		h, ok := m.(plugin.Middleware)
		if !ok {
			return fmt.Errorf("handler %d of subnet %s (%T) is not a plugin.Middleware", i, s.Network.String(), m)
		}
		handlers = append(handlers, h)
	}

	s.buildMiddlewareChain(handlers)

	return nil
}

// Cleanup closes the lease database. It is safe to call on a server that
// failed to provision.
func (s *Server) Cleanup() error {
	var err error
	if c, ok := s.store.(io.Closer); ok {
		err = c.Close()
	}

	s.store = nil
	s.Database = nil

	return err
}

func findInterface(s *Server) bool {
	if s.iface.Name != "" && len(s.iface.HardwareAddr) > 0 {
		return true
	}

	ifc, err := iface.ByIP(s.IP)
	if err != nil {
		return false
	}

	s.iface = *ifc
	return true
}

func (s *Server) buildMiddlewareChain(handlers []plugin.Middleware) {
	var endOfChainHandler plugin.HandlerFunc = func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
		peer := GetPeer(ctx)
		l := log.With(ctx, s.logger)

		// if it's a DHCPREQUEST that we didn't handle yet we will send
		// DHCPNAK
		if Request(req) {
			l.Warnf("unhandled DHCPREQUEST, responding with DHCPNAK")
			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
			return nil
		}

		l.Infof("%s from %s not handled. dropping", req.MessageType().String(), peer)
		return ErrNoResponse
	}

	var chain plugin.Handler = endOfChainHandler
	for i := len(handlers) - 1; i >= 0; i-- {
		chain = handlers[i].Wrap(chain)
		s.logger.Debugf("plugin (%d) %s setup", i, chain.Name())
	}

	s.chain = chain
}

// start opens the server's connection and starts serving requests from it.
func (s *Server) start() error {
	conn, err := listenDHCP(s.logger, s.IP, &s.iface)
	if err != nil {
		return fmt.Errorf("failed to listen on %s (%s): %w", s.IP, s.iface.Name, err)
	}
	s.conn = conn
	s.stopping.Store(false)

	s.serveWg.Add(1)
	go func() {
		defer s.serveWg.Done()

		err := s.ServePacket(conn)
		if errors.Is(err, net.ErrClosed) || s.stopping.Load() {
			return
		}

		err = fmt.Errorf("stopped serving subnet %s: %w", s.Network.String(), err)
		s.logger.Error(err.Error())
		s.reportFailure(err)
	}()

	return nil
}

// reportFailure sends err to the channel of failures, unless the error of
// another server is still waiting there.
func (s *Server) reportFailure(err error) {
	if s.failed == nil {
		return
	}

	select {
	case s.failed <- err:
	default:
	}
}

// stop closes the server's connection and waits for the requests being
// served to finish.
func (s *Server) stop() error {
	if s.conn == nil {
		return nil
	}

	s.stopping.Store(true)
	err := s.conn.Close()
	s.serveWg.Wait()
	s.dhcpWg.Wait()
	s.conn = nil

	return err
}

// ServePacket serves requests read from c until reading from it fails, for
// example because c was closed.
func (s *Server) ServePacket(c net.PacketConn) error {
	for {
		payload := make([]byte, 4096)
		byteLen, addr, err := c.ReadFrom(payload)

		if byteLen > 0 {
			s.logger.Debugf("serving request from %s", addr)
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

func (s *Server) serveAndLogDHCPv4(c net.PacketConn, payload []byte, addr net.Addr) {
	defer s.dhcpWg.Done()
	// In any case we must not panic while serving requests
	defer func() {
		if x := recover(); x != nil {
			s.logger.Infof("Caught panic while serving a DHCP request from %s", addr.String())
			s.logger.Infof("\t%v", x)
			s.logger.Infof("%s", debug.Stack())
		}
	}()

	err := s.serveDHCPv4(c, payload, addr)
	if err != nil {
		s.logger.Warnf("failed to serve request from %s: %s", addr.String(), err.Error())
	}
}

func (s *Server) findSubnetConfig(gwIP net.IP) *Server {
	return s
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
		s.logger.Debugf("ignoring packet with incorrect server ID %q from %s", reqID, msg.ClientHWAddr)
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
func updateConnectionAddresses(ctx context.Context, addr net.Addr, cfg *Server, req, resp *dhcpv4.DHCPv4) net.Addr {
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
			a.Local.MAC = cfg.iface.HardwareAddr
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

// Interface guards
var (
	_ caddy.Provisioner  = (*Server)(nil)
	_ caddy.CleanerUpper = (*Server)(nil)
)
