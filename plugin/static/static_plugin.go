package static

import (
	"context"
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
// address of the client is configured a static IP lease will be sent
func (s *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	static, hasStatic := s.Addresses[req.ClientHWAddr.String()]
	if (dhcpserver.Discover(req) || dhcpserver.Request(req)) && hasStatic {
		// Make sure to deny a DHCPREQUEST for a different IP address
		// for DHCPDISCOVER we can safely ignore the RequestedIPAddress field by RFC
		if dhcpserver.Request(req) {
			reqIP := req.RequestedIPAddress()
			if reqIP == nil || reqIP.IsUnspecified() {
				reqIP = req.ClientIPAddr
			}

			if reqIP.String() != static.String() {
				s.L.Warnf("%s: denying request for IP %s", req.ClientHWAddr.String(), reqIP)

				res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
				return nil
			}

			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
			s.storeLease(ctx, req, res, static)
		}

		// TODO(ppacher): should we allow configuration of leaseTime or client specific options here?
		res.YourIPAddr = static

		// TODO(ppacher): we may remove this and make setting the subnet mask a default action of dhcpserver.Server
		if req.IsOptionRequested(dhcpv4.OptionSubnetMask) {
			res.UpdateOption(dhcpv4.OptSubnetMask(s.Config.Network.Mask))
		}

		s.L.Infof("%s: serving static IP %s (%s)", req.ClientHWAddr, res.YourIPAddr, req.MessageType())
		return nil
	}

	return s.Next.ServeDHCP(ctx, req, res)
}

// storeLease stores the lease of the static IP address ip for the client of
// req, so that the address stays leased to the client until the lease
// expires if ip is removed from the configuration, and tells the client the
// lease time. Each request renews the lease, so the database lets it expire
// when the client is told. If storing it fails, the client is still sent ip,
// with the lease time configured.
func (s *Plugin) storeLease(ctx context.Context, req, res *dhcpv4.DHCPv4, ip net.IP) {
	cli := lease.NewClient(req.ClientHWAddr)

	leaseTime, err := s.Config.Database.Lease(ctx, ip, cli, res.IPAddressLeaseTime(time.Hour), true)
	if err != nil {
		log.With(ctx, s.L).Warnf("%s: failed to store the lease of static IP %s: %s", req.ClientHWAddr, ip, err.Error())
		return
	}

	res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
}
