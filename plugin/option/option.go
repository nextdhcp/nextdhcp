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
	"github.com/rs/zerolog"
)

// Plugin allows to configure and set arbitrary DHCP
// options. It implements the plugin.Handler interface
type Plugin struct {
	// OptionsRaw are the options to set, in the order they are configured.
	// An option replaces an earlier option with the same code.
	OptionsRaw []OptionConfig `json:"options,omitempty"`

	Next    plugin.Handler                           `json:"-"`
	Options map[dhcpv4.OptionCode]dhcpv4.OptionValue `json:"-"`
	L       zerolog.Logger                           `json:"-"`
}

// OptionConfig configures a DHCP option.
type OptionConfig struct {
	// Name is the name of the option, like "router", or its code, like
	// "0x03".
	Name string `json:"name"`

	// Values are the values of the option.
	Values []string `json:"values,omitempty"`
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
		v = strings.TrimPrefix(v, "0x")

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
