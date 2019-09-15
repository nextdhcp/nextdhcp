package option

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

// Plugin allows to configure and set arbitrary DHCP
// options. It implements the plugin.Handler interface
type Plugin struct {
	Next    plugin.Handler
	Options map[dhcpv4.OptionCode]dhcpv4.OptionValue
}

// Name implements the plugin.Handler interface and returns "option"
func (p *Plugin) Name() string {
	return "option"
}

// ServeDHCP implements the plugin.Handler interface and will add all configured DHCP options
// if they are requested
func (p *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	if dhcpserver.Discover(req) || dhcpserver.Request(req) {
		for code, value := range p.Options {
			if req.IsOptionRequested(code) {
				// TODO(ppacher): should we only set the option if no plugin above us already
				// did it?
				res.UpdateOption(dhcpv4.OptGeneric(code, value.ToBytes()))
			}
		}
	}

	return p.Next.ServeDHCP(ctx, req, res)
}

func (p *Plugin) parseOption(name string, values []string) error {
	c, v, err := ParseKnownOption(name, values)
	if err != nil {
		return err
	}

	p.Options[c] = v

	return nil
}
