package serverid

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type serverID struct {
	next plugin.Handler
	id   net.IP
	L    log.Logger
}

// Name returns "serverid" and implements plugin.Handler
func (*serverID) Name() string {
	return "serverid"
}

// ServeDHCP serves a DHCP request message and implements plugin.Handler
func (s *serverID) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	reqID := req.ServerIdentifier()
	// Drop it if it's not for us
	if reqID != nil && !reqID.IsUnspecified() && reqID.String() != s.id.String() {
		s.L.Debugf("ignoring packet with incorrect server ID %q from %s", req.ClientHWAddr, reqID)
		return dhcpserver.ErrNoResponse
	}

	res.UpdateOption(dhcpv4.OptServerIdentifier(s.id))

	return s.next.ServeDHCP(ctx, req, res)
}
