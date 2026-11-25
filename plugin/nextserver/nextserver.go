package nextserver

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type nextServerPlugin struct {
	// NextServer is the address of the server clients boot from, set as the
	// server address (siaddr) of responses.
	NextServer net.IP `json:"next_server,omitempty"`

	next plugin.Handler
}

func (*nextServerPlugin) Name() string {
	return "next-server"
}

func (s *nextServerPlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	res.ServerIPAddr = s.NextServer
	return s.next.ServeDHCP(ctx, req, res)
}
