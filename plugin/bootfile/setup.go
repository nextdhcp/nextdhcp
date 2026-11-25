package bootfile

import (
	"errors"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(Plugin{})
	dhcpserver.RegisterHandlerDirective("bootfile", setupBootFile)
}

// Plugin allows to configure and set arbitrary DHCP
// options. It implements the plugin.Handler interface
type Plugin struct {
	// Bootfile maps the boot modes "bios" and "uefi" to the boot file
	// clients that boot in that mode get.
	Bootfile map[BootMode]string `json:"bootfile,omitempty"`

	Next plugin.Handler `json:"-"`
	L    log.Logger     `json:"-"`
}

// CaddyModule returns the Caddy module information.
func (Plugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.bootfile",
		New: func() caddy.Module { return new(Plugin) },
	}
}

// Provision sets up the plugin's logger.
func (p *Plugin) Provision(ctx caddy.Context) error {
	if p.Bootfile == nil {
		p.Bootfile = make(map[BootMode]string)
	}

	p.L = ctx.Logger().Sugar()

	return nil
}

// Wrap implements plugin.Middleware.
func (p *Plugin) Wrap(next plugin.Handler) plugin.Handler {
	p.Next = next
	return p
}

func setupBootFile(h dhcpserver.Helper) (caddy.Module, error) {
	p := new(Plugin)
	return p, p.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all bootfile directives of a
// server block.
func (p *Plugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	p.Bootfile = make(map[BootMode]string)

	for c.Next() {
		if c.NextBlock(0) {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}
			err := p.parseBootFile(name, values)
			if err != nil {
				return err
			}

			for c.NextBlock(0) {
				name = c.Val()
				values = c.RemainingArgs()
				if len(values) == 0 {
					return c.ArgErr()
				}
				err := p.parseBootFile(name, values)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (p *Plugin) parseBootFile(name string, values []string) error {
	if len(values) > 1 {
		return errors.New("bootfile only surport one value for each boot mode")
	}
	switch strings.ToLower(name) {
	case "bios":
		p.Bootfile[BIOS] = values[0]
	case "legacy":
		p.Bootfile[BIOS] = values[0]
	case "uefi":
		p.Bootfile[UEFI] = values[0]
	default:
		return errors.New("unknown boot mode")
	}
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*Plugin)(nil)
	_ caddyfile.Unmarshaler = (*Plugin)(nil)
	_ plugin.Middleware     = (*Plugin)(nil)
)
