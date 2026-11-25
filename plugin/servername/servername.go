package servername

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type serverNamePlugin struct {
	// ServerName is the server host name set on responses.
	ServerName string `json:"server_name,omitempty"`

	next plugin.Handler
}

func (*serverNamePlugin) Name() string {
	return "servername"
}

func (s *serverNamePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	res.ServerHostName = s.ServerName
	return s.next.ServeDHCP(ctx, req, res)
}
