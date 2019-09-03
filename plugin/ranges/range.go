package ranges

import (
	"context"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/iprange"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("range", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupRange,
	})
}

type rangePlugin struct {
	next   plugin.Handler
	ranges iprange.IPRanges
}

func (p *rangePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	db := lease.GetDatabase(ctx)

	// we only serve discover and request message types
	if dhcpserver.Discover(req) || dhcpserver.Request(req) {
	}

	// If it's a DHCPRELEASE message and part of our range we'll release it
	if dhcpserver.Release(req) && p.ranges.Contains(req.ClientIPAddr) {
		if err := db.Release(ctx, req.ClientIPAddr); err != nil {
			return err
		}

		// No response should be sent for DHCPRELEASE messages
		return dhcpserver.ErrNoResponse
	}

	return p.next.ServeDHCP(ctx, req, res)
}

func (p *rangePlugin) Name() string {
	return "ranges"
}

func setupRange(c *caddy.Controller) error {
	plg := &rangePlugin{}

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

		r := &iprange.IPRange{
			Start: startIP,
			End:   endIP,
		}

		plg.ranges = append(plg.ranges, r)
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.next = next
		return plg
	})

	return nil
}
