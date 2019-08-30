package lease

import (
	"time"
	"github.com/caddyserver/caddy"
	"github.com/ppacher/dhcp-ng/core/dhcpserver"
)

func init() {
	caddy.RegisterPlugin("lease", caddy.Plugin{
		ServerType: "dhcpv4",
		Action: setupLease,	
	})
}

func setupLease(c *caddy.Controller) error {
	config := dhcpserver.GetConfig(c)

	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}
		
		d, err := time.ParseDuration(c.Val())
		if err != nil {
			return c.SyntaxErr("time.Duration")
		}
		
		config.LeaseTime = d
	}

	return nil
}