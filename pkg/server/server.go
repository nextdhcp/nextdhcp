package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease"
	"golang.org/x/sync/errgroup"
)

// Server is a DHCPv4 server
type Server interface {
	// Wait for the server to finish and returns the first error encoutnered
	Wait() error

	// Start starts serving incoming DHCP requests
	Start(ctx context.Context) error
}

type server struct {
	listens []net.IP // array of addresses to listen
	conns   []net.PacketConn

	provider lease.Database

	grp *errgroup.Group
}

// Option is a server option and use to configure the DHCP4 server
type Option func(s *server)

// WithListen configures one or more listen addresses for the DHCP server
func WithListen(l ...net.IP) Option {
	return func(s *server) {
		s.listens = append(s.listens, l...)
	}
}

// New creates a new DHCPv4 server
func New(opts ...Option) Server {
	_, network, _ := net.ParseCIDR("10.100.0.1/24")
	s := &server{
		provider: lease.NewDatabase(network, nil),
	}

	s.provider.AddRange(
		&lease.IPRange{
			Start: net.ParseIP("10.100.0.100"),
			End:   net.ParseIP("10.100.0.150"),
		},
	)

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *server) Start(ctx context.Context) error {
	s.grp, ctx = errgroup.WithContext(ctx)

	errs := make(chan error, 1)
	s.grp.Go(func() error {
		return <-errs
	})

	for _, l := range s.listens {
		conn, err := NewListener(l)
		if err != nil {
			errs <- err
			return err
		}

		s.serveConn(ctx, conn)
	}

	errs <- nil
	return nil
}

func (s *server) serveConn(ctx context.Context, conn Listener) {
	log.Printf("Starting to serve on %s", conn.LocalAddr().String())
	go func() {
		s.grp.Wait()
		log.Printf("Closed connection %s", conn.LocalAddr().String())
		conn.Close()
	}()

	s.grp.Go(func() error {
		defer log.Printf("Stopped serving %s", conn.LocalAddr().String())
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			deadline := time.Now().Add(1 * time.Second)
			if err := conn.SetReadDeadline(deadline); err != nil {
				return err
			}

			buf := make([]byte, 4096)
			n, peer, err := conn.ReadFrom(buf)

			if err != nil {
				// Read timeouts and temporary network errors are fine
				if opErr, ok := err.(*net.OpError); ok {
					if opErr.Timeout() {
						continue
					}

					if opErr.Temporary() {
						log.Printf("Temporary network error: %s", opErr.Error())
						continue
					}
				}

				return err
			}

			msg, err := dhcpv4.FromBytes(buf[:n])
			if err != nil {
				log.Printf("Failed to parse DHCPv4 message: %s", err.Error())
				continue
			}

			peerAddr, ok := peer.(*net.UDPAddr)
			if !ok {
				log.Printf("Not a UDP connection? Peer is %s", peer.String())

				// default to the broadcast IP if we operate on a non-UDP connection
				peerAddr = &net.UDPAddr{
					IP:   net.IPv4bcast,
					Port: 0,
				}
			}

			if peerAddr.IP == nil || peerAddr.IP.Equal(net.IPv4zero) {
				peerAddr = &net.UDPAddr{
					IP:   net.IPv4bcast,
					Port: peerAddr.Port,
				}
			}

			log.Printf("got request from %s: %s", peerAddr.String(), msg.ClientHWAddr.String())
			log.Printf(msg.Summary())
			// TODO(ppacher): handle request

			switch msg.MessageType() {
			case dhcpv4.MessageTypeDiscover:
				err = s.handleDiscovery(conn, peer, msg)
			case dhcpv4.MessageTypeRequest:
				err = s.handleRequest(conn, peer, msg)
			default:
				err = errors.New("unsupported message type")
			}

			if err != nil {
				log.Printf("failed to handle DHCP request: %s", err)
			}
		}
	})
}

// https://tools.ietf.org/html/rfc2131#section-4.3.1
func (s *server) handleDiscovery(conn Listener, peer net.Addr, msg *dhcpv4.DHCPv4) error {
	cli := lease.Client{
		HwAddr:   msg.ClientHWAddr,
		Hostname: msg.HostName(),
	}

	var ip net.IP
	var err error

	// TODO(ppacher): if RequestedIPAddress != nil try to reserve that one

	if ip, err = s.provider.FindAddress(context.Background(), &cli); err != nil {
		return err
	}

	log.Printf("Offering IP %s to %s", ip, cli)
	if err := s.provider.Reserve(context.Background(), ip, cli); err != nil {
		log.Printf("failed to reserve IP address %s for %s: %s", ip, cli, err)
		return err
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(conn.IP()),
	)
	if err != nil {
		return err
	}

	resp.UpdateOption(dhcpv4.OptServerIdentifier(conn.IP()))
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	resp.UpdateOption(dhcpv4.OptSubnetMask(net.IPv4Mask(255, 255, 255, 0)))
	resp.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Minute))
	//resp.UpdateOption(dhcpv4.OptRouter(conn.IP()))
	//resp.UpdateOption(dhcpv4.OptDNS(net.IP{8, 8, 8, 8}))

	if !msg.GatewayIPAddr.IsUnspecified() {
		// TODO: make RFC8357 compliant
		peer = &net.UDPAddr{IP: msg.GatewayIPAddr, Port: dhcpv4.ClientPort}

		_, err := conn.WriteTo(resp.ToBytes(), peer)
		return err
	}

	log.Println("Response ====")
	log.Println(resp.Summary())

	return conn.SendRaw(resp.YourIPAddr, resp.ClientHWAddr, resp.ToBytes())
}

func (s *server) handleRequest(conn Listener, peer net.Addr, msg *dhcpv4.DHCPv4) error {
	cli := lease.Client{
		HwAddr:   msg.ClientHWAddr,
		Hostname: msg.HostName(),
	}
	ip := msg.RequestedIPAddress()

	if ip == nil {
		ip = msg.ClientIPAddr
	}

	renewing := msg.ClientIPAddr != nil

	leaseTime, err := s.provider.Lease(context.Background(), ip, cli, time.Minute, renewing)
	if err != nil {
		// should we send a DHCPNACK?
		return fmt.Errorf("failed to lease %s: %s", ip, err.Error())
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(conn.IP()),
	)
	if err != nil {
		return err
	}

	resp.UpdateOption(dhcpv4.OptServerIdentifier(conn.IP()))
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	resp.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
	resp.UpdateOption(dhcpv4.OptSubnetMask(net.IPv4Mask(255, 255, 255, 0)))

	if !msg.GatewayIPAddr.IsUnspecified() {
		// TODO: make RFC8357 compliant
		peer = &net.UDPAddr{IP: msg.GatewayIPAddr, Port: dhcpv4.ClientPort}

		_, err := conn.WriteTo(resp.ToBytes(), peer)
		return err
	}

	log.Println("Response ====")
	log.Println(resp.Summary())

	return conn.SendRaw(resp.YourIPAddr, resp.ClientHWAddr, resp.ToBytes())
}

func (s *server) Wait() error {
	return s.grp.Wait()
}

func (s *server) listenUDP(addr string) (net.PacketConn, error) {
	udp, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", udp)
	if err != nil {
		return nil, err
	}

	return conn, err
}
