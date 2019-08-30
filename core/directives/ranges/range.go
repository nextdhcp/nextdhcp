package ranges

import (
	"net"
	"github.com/caddyserver/caddy"
	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
	"github.com/ppacher/dhcp-ng/core/dhcpserver"
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

	return nil
}