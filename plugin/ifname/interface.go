package ifname

import (
	"fmt"
	"net"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
)

func init() {
	dhcpserver.RegisterDirective("interface", setupInterface)
}

func setupInterface(c dhcpserver.Helper) error {
	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		iface, err := net.InterfaceByName(c.Val())
		if err != nil {
			return fmt.Errorf("failed to find interface with name %s: %s", c.Val(), err.Error())
		}

		c.Server.Interface = iface.Name
	}

	return nil
}
