package server

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
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
	listens []string // array of addresses to listen
	conns   []net.PacketConn

	grp *errgroup.Group
}

// Option is a server option and use to configure the DHCP4 server
type Option func(s *server)

// WithConn sets the UDP packet connection to use
func WithConn(conn net.PacketConn) Option {
	return func(s *server) {
		s.conns = append(s.conns, conn)
	}
}

// WithListen configures one or more listen addresses for the DHCP server
func WithListen(l ...string) Option {
	return func(s *server) {
		s.listens = append(s.listens, l...)
	}
}

// New creates a new DHCPv4 server
func New(opts ...Option) Server {
	s := &server{}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *server) Start(ctx context.Context) error {
	s.grp, ctx = errgroup.WithContext(ctx)

	for _, l := range s.listens {
		conn, err := s.listenUDP(l)
		if err != nil {
			return err
		}

		s.conns = append(s.conns, conn)
	}

	for _, c := range s.conns {
		s.serveConn(ctx, c)
	}

	return nil
}

func (s *server) serveConn(ctx context.Context, conn net.PacketConn) {
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
		}
	})
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
