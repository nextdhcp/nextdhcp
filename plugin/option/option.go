package option

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/option"
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
		bootFile, err := p.GetBootFileOpt(ctx, req, res)
		if err != nil {
			return err
		}
		res.UpdateOption(*bootFile)
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
	c, v, err := option.ParseKnown(name, values)
	if err != nil {
		if err != option.ErrUnknownOption {
			return err
		}

		// check if we can parse a custom option
		c, v, err = parseCustomOption(name, values)
		if err != nil {
			return err
		}
	}

	p.Options[c] = v

	return nil
}

func parseCustomOption(name string, values []string) (dhcpv4.OptionCode, dhcpv4.OptionValue, error) {
	// ParseUint handles octal, hex and binary values as well so let's just try to get a byte option
	// code
	code, err := strconv.ParseUint(name, 0, 8)
	if err != nil {
		return nil, nil, err
	}

	var payloads [][]byte
	for _, v := range values {
		if strings.HasPrefix(v, "0x") {
			v = strings.TrimPrefix(v, "0x")
		}

		b, err := hex.DecodeString(v)
		if err != nil {
			return nil, nil, err
		}

		payloads = append(payloads, b)
	}

	// now merge the payloads into one byte slice
	value := dhcpv4.OptionGeneric{}
	for _, p := range payloads {
		value.Data = append(value.Data, p...)
	}

	return dhcpv4.GenericOptionCode(code), value, nil
}
