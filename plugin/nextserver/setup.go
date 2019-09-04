package nextserver

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("next-server", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupNextServer,
	})
}

func setupNextServer(c *caddy.Controller) error {
	if !c.Next() {
		return c.ArgErr()
	}

	if !c.NextArg() {
		return c.ArgErr()
	}

	serverName := c.Val()
	ip := net.ParseIP(serverName)
	if ip == nil {
		return fmt.Errorf("%s:%d: expected IP address but got %s", c.File(), c.Line(), serverName)
	}

	if len(c.RemainingArgs()) > 0 || c.Next() {
		return c.ArgErr()
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &nextServerPlugin{
			next:       next,
			nextServer: ip,
		}
	})

	return nil
}
