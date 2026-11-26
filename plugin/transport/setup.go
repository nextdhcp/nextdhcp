// Package transport implements the transport directive, which sets how a
// server exchanges messages with clients.
package transport

import (
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
)

func init() {
	dhcpserver.RegisterDirective("transport", setupTransport)
}

func setupTransport(c dhcpserver.Helper) error {
	set := false

	for c.Next() {
		if set {
			return c.Err("transport may only be set once per server block")
		}
		set = true

		if !c.NextArg() {
			return c.ArgErr()
		}
		transport := c.Val()

		if c.NextArg() {
			return c.ArgErr()
		}

		switch transport {
		case dhcpserver.TransportPacket, dhcpserver.TransportUDP:
		default:
			return c.Errf("unknown transport %q: it must be %q or %q", transport, dhcpserver.TransportPacket, dhcpserver.TransportUDP)
		}

		c.Server.Transport = transport
	}

	return nil
}
