package serverid

import (
	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("serverid", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupServerID,
	})
}

func setupServerID(c *caddy.Controller) error {
	c.Next()

	if c.NextArg() {
		return c.ArgErr()
	}

	if c.Next() {
		return c.Err("serverid can only be set once")
	}

	cfg := dhcpserver.GetConfig(c)

	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &serverID{
			next: next,
			id:   cfg.IP,
		}
	})

	return nil
}
