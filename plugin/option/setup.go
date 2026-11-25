package option

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(Plugin{})
	dhcpserver.RegisterHandlerDirective("option", setupOption)
}

// CaddyModule returns the Caddy module information.
func (Plugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.option",
		New: func() caddy.Module { return new(Plugin) },
	}
}

// Provision parses the configured options.
func (p *Plugin) Provision(ctx caddy.Context) error {
	p.Options = make(map[dhcpv4.OptionCode]dhcpv4.OptionValue)
	p.L = ctx.Logger().Sugar()

	for _, o := range p.OptionsRaw {
		if err := p.parseOption(o.Name, o.Values); err != nil {
			return err
		}
	}

	return nil
}

// Wrap implements plugin.Middleware.
func (p *Plugin) Wrap(next plugin.Handler) plugin.Handler {
	p.Next = next
	return p
}

func setupOption(h dhcpserver.Helper) (caddy.Module, error) {
	plg := new(Plugin)
	return plg, plg.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all option directives of a
// server block.
func (p *Plugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	p.Options = make(map[dhcpv4.OptionCode]dhcpv4.OptionValue)

	for c.Next() {
		if c.NextBlock(0) {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}

			if err := p.addOption(name, values); err != nil {
				return err
			}

			for c.NextBlock(0) {
				name = c.Val()
				values = c.RemainingArgs()
				if len(values) == 0 {
					return c.ArgErr()
				}

				if err := p.addOption(name, values); err != nil {
					return err
				}
			}

		} else if c.NextArg() {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}

			if err := p.addOption(name, values); err != nil {
				return err
			}
		}
	}

	return nil
}

// addOption checks and adds an option to the configured options.
func (p *Plugin) addOption(name string, values []string) error {
	if err := p.parseOption(name, values); err != nil {
		return err
	}

	p.OptionsRaw = append(p.OptionsRaw, OptionConfig{Name: name, Values: values})

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*Plugin)(nil)
	_ caddyfile.Unmarshaler = (*Plugin)(nil)
	_ plugin.Middleware     = (*Plugin)(nil)
)
