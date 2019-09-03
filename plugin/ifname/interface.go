package ifname

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/ppacher/dhcp-ng/core/dhcpserver"
)

func init() {
	caddy.RegisterPlugin("interface", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupInterface,
	})
}

func setupInterface(c *caddy.Controller) error {
	config := dhcpserver.GetConfig(c)

	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		iface, err := net.InterfaceByName(c.Val())
		if err != nil {
			return fmt.Errorf("failed to find interface with name %s: %s", c.Val(), err.Error())
		}
		
		config.Interface = *iface
	}

	return nil
}
