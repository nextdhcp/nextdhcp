package static

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("static", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupStatic,
	})
}

func setupStatic(c *caddy.Controller) error {
	plg, err := makeStaticPlugin(c)
	if err != nil {
		return err
	}

	plg.Config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.Next = next
		return plg
	})
	return nil
}

func makeStaticPlugin(c *caddy.Controller) (*Plugin, error) {
	addr := make(map[string]net.IP)
	ips := make(map[string]struct{})

	for c.Next() {
		if !c.NextArg() {
			return nil, c.ArgErr()
		}

		key := c.Val()
		if _, err := net.ParseMAC(key); err != nil {
			return nil, c.ArgErr()
		}

		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		ip := net.ParseIP(c.Val())
		if ip == nil {
			return nil, c.ArgErr()
		}

		if e, ok := addr[key]; ok {
			return nil, fmt.Errorf("Static IP address %s has already been configured for client %s", e.String(), key)
		}

		if _, ok := ips[ip.String()]; ok {
			return nil, fmt.Errorf("IP %s already used for client %s", ip, key)
		}

		addr[key] = ip
		ips[ip.String()] = struct{}{}
	}

	plg := &Plugin{
		Addresses: addr,
		Config:    dhcpserver.GetConfig(c),
	}

	return plg, nil
}
