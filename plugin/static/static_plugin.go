package static

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

// Plugin allows assignment of static IP addresses to clients
// based on the MAC address. It implements plugin.Handler
type Plugin struct {
	// Addresses maps client MAC addresses to the IP addresses they are
	// leased.
	Addresses map[string]net.IP `json:"addresses,omitempty"`

	Config *dhcpserver.Server `json:"-"`
	Next   plugin.Handler     `json:"-"`
	L      log.Logger         `json:"-"`
}

// Name returns "static" and implements plugin.Handler
func (s *Plugin) Name() string {
	return "static"
}

// ServeDHCP serves a DHCP request and implements plugin.Handler. If the requesting MAC
// address of the client is configured a static IP lease will be sent. While
// another client still has a lease of the static IP address, from before it
// was configured, the request is passed on to the next handler, so that the
// client is served like any other until that lease ends.
func (s *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	static, hasStatic := s.Addresses[req.ClientHWAddr.String()]
	if !hasStatic || !dhcpserver.Discover(req) && !dhcpserver.Request(req) {
		return s.Next.ServeDHCP(ctx, req, res)
	}

	l := log.With(ctx, s.L)
	cli := lease.NewClient(req.ClientHWAddr)

	if dhcpserver.Request(req) {
		reqIP := req.RequestedIPAddress()
		if reqIP == nil || reqIP.IsUnspecified() {
			reqIP = req.ClientIPAddr
		}

		// Deny a DHCPREQUEST for a different IP address, so that the client
		// asks for its static address, unless that is still leased to
		// another client. For DHCPDISCOVER we can safely ignore the
		// RequestedIPAddress field by RFC
		if reqIP.String() != static.String() {
			if s.inUse(ctx, static, cli) {
				return s.Next.ServeDHCP(ctx, req, res)
			}

			l.Warnf("%s: denying request for IP %s", req.ClientHWAddr.String(), reqIP)

			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
			return nil
		}

		// Store the lease, so that the address stays leased to the client
		// until the lease expires if it is removed from the configuration.
		// Each request renews the lease, so the database lets it expire when
		// the client is told. If storing it fails, the client is still sent
		// its address, with the lease time configured
		leaseTime, err := s.Config.Database.Lease(ctx, static, cli, res.IPAddressLeaseTime(time.Hour), true)
		switch {
		case errors.Is(err, lease.ErrAddressReserved):
			l.Infof("%s: static IP %s is still leased to another client", req.ClientHWAddr, static)
			return s.Next.ServeDHCP(ctx, req, res)
		case err != nil:
			l.Warnf("%s: failed to store the lease of static IP %s: %s", req.ClientHWAddr, static, err.Error())
		default:
			res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
		}

		res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	} else if s.inUse(ctx, static, cli) {
		l.Infof("%s: static IP %s is still leased to another client", req.ClientHWAddr, static)
		return s.Next.ServeDHCP(ctx, req, res)
	}

	// TODO(ppacher): should we allow configuration of leaseTime or client specific options here?
	res.YourIPAddr = static

	// TODO(ppacher): we may remove this and make setting the subnet mask a default action of dhcpserver.Server
	if req.IsOptionRequested(dhcpv4.OptionSubnetMask) {
		res.UpdateOption(dhcpv4.OptSubnetMask(s.Config.Network.Mask))
	}

	l.Infof("%s: serving static IP %s (%s)", req.ClientHWAddr, res.YourIPAddr, req.MessageType())
	return nil
}

// inUse reports whether the static IP address ip of cli is still leased to
// another client, which may use it until that lease ends. If the database
// fails to tell, the client gets its address.
func (s *Plugin) inUse(ctx context.Context, ip net.IP, cli lease.Client) bool {
	err := s.Config.Database.Reserve(ctx, ip, cli)
	if err != nil && !errors.Is(err, lease.ErrAddressReserved) {
		log.With(ctx, s.L).Warnf("%s: failed to check whether static IP %s is leased to another client: %s", cli.HwAddr, ip, err.Error())
	}

	return errors.Is(err, lease.ErrAddressReserved)
}
