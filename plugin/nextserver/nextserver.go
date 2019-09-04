package nextserver

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type nextServerPlugin struct {
	next       plugin.Handler
	nextServer net.IP
}

func (*nextServerPlugin) Name() string {
	return "next-server"
}

func (s *nextServerPlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	res.ServerIPAddr = s.nextServer
	return s.next.ServeDHCP(ctx, req, res)
}
