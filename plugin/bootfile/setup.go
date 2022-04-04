package bootfile

import (
	"errors"
	"strings"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("bootfile", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupBootFile,
	})
}

// Plugin allows to configure and set arbitrary DHCP
// options. It implements the plugin.Handler interface
type Plugin struct {
	Next     plugin.Handler
	Bootfile map[BootMode]string
	L        log.Logger
}

func setupBootFile(c *caddy.Controller) error {
	p := &Plugin{
		Bootfile: make(map[BootMode]string),
	}
	for c.Next() {
		if c.NextBlock() {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}
			err := p.parseBootFile(name, values)
			
			if err != nil {
				return err
			}

			for c.NextBlock() {
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

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		p.Next = next
		return p
	})
	p.L = log.GetLogger(c, p)
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
