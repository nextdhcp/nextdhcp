package nextserver

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(nextServerPlugin{})
	dhcpserver.RegisterHandlerDirective("next-server", setupNextServer)
}

// CaddyModule returns the Caddy module information.
func (nextServerPlugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.next_server",
		New: func() caddy.Module { return new(nextServerPlugin) },
	}
}

// Wrap implements plugin.Middleware.
func (s *nextServerPlugin) Wrap(next plugin.Handler) plugin.Handler {
	s.next = next
	return s
}

func setupNextServer(h dhcpserver.Helper) (caddy.Module, error) {
	p := new(nextServerPlugin)
	return p, p.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from the next-server directive of a
// server block.
func (s *nextServerPlugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	if !c.Next() {
		return c.ArgErr()
	}

	if !c.NextArg() {
		return c.ArgErr()
	}

	serverName := c.Val()
	ip := net.ParseIP(serverName)
	if ip == nil {
		return fmt.Errorf("%s:%d: expected IP address but got %s", c.File(), c.Line(), serverName)
	}

	if len(c.RemainingArgs()) > 0 || c.Next() {
		return c.ArgErr()
	}

	s.NextServer = ip

	return nil
}

// Interface guards
var (
	_ caddyfile.Unmarshaler = (*nextServerPlugin)(nil)
	_ plugin.Middleware     = (*nextServerPlugin)(nil)
)
