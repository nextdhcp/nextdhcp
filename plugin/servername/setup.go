package servername

import (
	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("servername", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupServerName,
	})
}

func setupServerName(c *caddy.Controller) error {
	if !c.Next() {
		return c.ArgErr()
	}

	if !c.NextArg() {
		return c.ArgErr()
	}

	serverName := c.Val()

	if len(c.RemainingArgs()) > 0 || c.Next() {
		return c.ArgErr()
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &serverNamePlugin{
			next: next,
			name: serverName,
		}
	})

	return nil
}
