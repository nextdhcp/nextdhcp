package ranges

import (
	"net"
	"context"
	"log"
	
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/core/lease/iprange"
	"github.com/ppacher/dhcp-ng/core/dhcpserver"
	"github.com/ppacher/dhcp-ng/plugin"
)

func init() {
	caddy.RegisterPlugin("range", caddy.Plugin{
		ServerType: "dhcpv4",
		Action: setupRange,
	})
}

func setupRange(c *caddy.Controller) error {
	config := dhcpserver.GetConfig(c)
	
	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}
		
		startIP := net.ParseIP(c.Val())
		if startIP == nil {
			return c.SyntaxErr("IPv4 address")
		}
		
		if !c.NextArg() {
			return c.ArgErr()
		}
		
		endIP := net.ParseIP(c.Val())
		if endIP == nil {
			return c.SyntaxErr("IPv4 address")
		}	
		
		r := &iprange.IPRange{startIP, endIP}
		
		config.Ranges = append(config.Ranges, r)
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return plugin.HandlerFunc(func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
			log.Println("range plugin running")

			return next.ServeDHCP(ctx, req, res)
		})
	})

	return nil
}