package serverid

import (
	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
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

	plg := &serverID{
		id: cfg.IP,
	}
	plg.L = log.GetLogger(c, plg)

	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.next = next
		return plg
	})

	return nil
}
