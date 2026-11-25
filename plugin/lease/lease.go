package lease

import (
	"context"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(leaseTimePlugin{})
	dhcpserver.RegisterHandlerDirective("lease", setupLease)
}

type leaseTimePlugin struct {
	// LeaseTime is the lease time offered to clients, unless a handler before
	// this one already set it.
	LeaseTime caddy.Duration `json:"lease_time,omitempty"`

	next plugin.Handler
}

// CaddyModule returns the Caddy module information.
func (leaseTimePlugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.lease",
		New: func() caddy.Module { return new(leaseTimePlugin) },
	}
}

// Wrap implements plugin.Middleware.
func (p *leaseTimePlugin) Wrap(next plugin.Handler) plugin.Handler {
	p.next = next
	return p
}

func (p *leaseTimePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	if dhcpserver.Discover(req) || dhcpserver.Request(req) {
		code := dhcpv4.OptionIPAddressLeaseTime.Code()
		if _, ok := res.Options[code]; !ok {
			res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Duration(p.LeaseTime)))
		}
	}

	return p.next.ServeDHCP(ctx, req, res)
}

func (p *leaseTimePlugin) Name() string {
	return "lease"
}

func setupLease(h dhcpserver.Helper) (caddy.Module, error) {
	p := new(leaseTimePlugin)
	return p, p.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all lease directives of a
// server block.
func (p *leaseTimePlugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		// Unlike time.ParseDuration, caddy.ParseDuration supports days
		d, err := caddy.ParseDuration(c.Val())
		if err != nil {
			return c.SyntaxErr("a duration, like 1h or 1d")
		}

		p.LeaseTime = caddy.Duration(d)
	}

	return nil
}

// Interface guards
var (
	_ caddyfile.Unmarshaler = (*leaseTimePlugin)(nil)
	_ plugin.Middleware     = (*leaseTimePlugin)(nil)
)
