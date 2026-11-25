package dhcpserver

import (
	"context"
	"errors"
	"net"

	"github.com/caddyserver/caddy/v2"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

// ErrNoResponse is returned by plugins if no response should be sent to the client
// This may be used for DHCPRELEASE messages or by middleware handlers that filtered
// the request. It's not an actual error
var ErrNoResponse = errors.New("no response should be sent")

// Request checks if msg is a DHCPREQUEST
func Request(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeRequest
}

// Ack checks if msg is a DHCPACK
func Ack(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeAck
}

// Nak checks if msg is a DHCPNAK
func Nak(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeNak
}

// Decline checks if msg is a DHCPDECLINE
func Decline(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeDecline
}

// Offer checks if msg is a DHCPOFFER
func Offer(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeOffer
}

// Discover checks if msg is a DHCPDISCOVER
func Discover(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeDiscover
}

// Release checks if msg is DHCPRELEASE
func Release(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeRelease
}

// Inform checks if msg is a DHCPINFORM
func Inform(msg *dhcpv4.DHCPv4) bool {
	return msg.MessageType() == dhcpv4.MessageTypeInform
}

// PeerKey is the key used to associate a net.Addr with a
// context.Context
type PeerKey struct{}

// GetPeer returns the peer address associated with ctx
func GetPeer(ctx context.Context) net.Addr {
	val := ctx.Value(PeerKey{})
	return val.(net.Addr)
}

// WithPeer associates a peer addr with the ctx
func WithPeer(ctx context.Context, peer net.Addr) context.Context {
	return context.WithValue(ctx, PeerKey{}, peer)
}

type serverKey struct{}

// WithServer returns a copy of ctx that carries s. Servers provision their
// handlers with such a context.
func WithServer(ctx caddy.Context, s *Server) caddy.Context {
	return ctx.WithValue(serverKey{}, s)
}

// ServerFromContext returns the server that ctx carries, or nil. Handlers
// use it in Provision to get the server whose chain they are part of.
func ServerFromContext(ctx context.Context) *Server {
	s, _ := ctx.Value(serverKey{}).(*Server)
	return s
}
