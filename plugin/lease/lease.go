package lease

import (
	"context"
	"time"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("lease", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupLease,
	})
}

type leaseTimePlugin struct {
	next      plugin.Handler
	leaseTime time.Duration
}

func (p *leaseTimePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	if dhcpserver.Discover(req) || dhcpserver.Request(req) {
		code := dhcpv4.OptionIPAddressLeaseTime.Code()
		if _, ok := res.Options[code]; !ok {
			res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(p.leaseTime))
		}
	}

	return p.next.ServeDHCP(ctx, req, res)
}

func (p *leaseTimePlugin) Name() string {
	return "lease"
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

	config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &leaseTimePlugin{
			next:      next,
			leaseTime: config.LeaseTime,
		}
	})

	return nil
}
