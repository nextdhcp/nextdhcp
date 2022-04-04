package option

import (
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("option", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupOption,
	})
}

func setupOption(c *caddy.Controller) error {
	plg := &Plugin{
		Options: make(map[dhcpv4.OptionCode]dhcpv4.OptionValue),
	}

	for c.Next() {
		if c.NextBlock() {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}

			if err := plg.parseOption(name, values); err != nil {
				return err
			}

			for c.NextBlock() {
				name = c.Val()
				values = c.RemainingArgs()
				if len(values) == 0 {
					return c.ArgErr()
				}

				if err := plg.parseOption(name, values); err != nil {
					return err
				}
			}

		} else if c.NextArg() {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}

			if err := plg.parseOption(name, values); err != nil {
				return err
			}
		}
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.Next = next
		return plg
	})
	return nil
}
