package servername

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type serverNamePlugin struct {
	next plugin.Handler
	name string
}

func (*serverNamePlugin) Name() string {
	return "servername"
}

func (s *serverNamePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	res.ServerHostName = s.name
	return s.next.ServeDHCP(ctx, req, res)
}
