package static

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("static", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupStatic,
	})
}

func setupStatic(c *caddy.Controller) error {
	addr := make(map[string]net.IP)

	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		key := c.Val()
		if _, err := net.ParseMAC(key); err != nil {
			return c.ArgErr()
		}

		if !c.NextArg() {
			return c.ArgErr()
		}
		ip := net.ParseIP(c.Val())
		if ip == nil {
			return c.ArgErr()
		}

		if e, ok := addr[key]; ok {
			return fmt.Errorf("Static IP address %s has already been configured for client %s", e.String(), key)
		}

		addr[key] = ip
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg := &StaticPlugin{
			Next:      next,
			Addresses: addr,
		}

		plg.L = log.GetLogger(c, plg)

		return plg
	})
	return nil
}
