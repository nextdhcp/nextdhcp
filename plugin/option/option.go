package option

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type optionPlugin struct {
	next    plugin.Handler
	options map[dhcpv4.OptionCode]dhcpv4.OptionValue
}

// Name implements the plugin.Handler interface and returns option
func (p *optionPlugin) Name() string {
	return "option"
}

// ServeDHCP implements the plugin.Handler interface and will add all configured DHCP options
// if they are requested
func (p *optionPlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	if dhcpserver.Discover(req) || dhcpserver.Request(req) {
		for code, value := range p.options {
			if req.IsOptionRequested(code) {
				res.UpdateOption(dhcpv4.OptGeneric(code, value.ToBytes()))
			}
		}
	}

	return p.next.ServeDHCP(ctx, req, res)
}

func (p *optionPlugin) parseOption(name string, values []string) error {
	c, v, err := ParseKnownOption(name, values)
	if err != nil {
		return err
	}

	p.options[c] = v

	return nil
}
