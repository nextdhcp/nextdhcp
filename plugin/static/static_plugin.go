package static

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

// Plugin allows assignment of static IP addresses to clients
// based on the MAC address. It implements plugin.Handler
type Plugin struct {
	Config    *dhcpserver.Config
	Next      plugin.Handler
	Addresses map[string]net.IP
	L         log.Logger
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
				return s.Next.ServeDHCP(ctx, req, res)
			}

			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
		}
		if dhcpserver.Discover(req) {
			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
		}

		// TODO(ppacher): should we allow configuration of leaseTime or client specific options here?
		res.YourIPAddr = static

		// TODO(ppacher): we may remove this and make setting the subnet mask a default action of dhcpserver.Server
		if req.IsOptionRequested(dhcpv4.OptionSubnetMask) {
			req.UpdateOption(dhcpv4.OptSubnetMask(s.Config.Network.Mask))
		}

		s.L.Infof("%s: serving static IP %s (%s)", req.ClientHWAddr, res.YourIPAddr, req.MessageType())
		return s.Next.ServeDHCP(ctx, req, res)
	}

	return s.Next.ServeDHCP(ctx, req, res)
}
