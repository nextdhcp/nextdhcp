package servername

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(serverNamePlugin{})
	dhcpserver.RegisterHandlerDirective("servername", setupServerName)
}

// CaddyModule returns the Caddy module information.
func (serverNamePlugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.servername",
		New: func() caddy.Module { return new(serverNamePlugin) },
	}
}

// Wrap implements plugin.Middleware.
func (s *serverNamePlugin) Wrap(next plugin.Handler) plugin.Handler {
	s.next = next
	return s
}

func setupServerName(h dhcpserver.Helper) (caddy.Module, error) {
	p := new(serverNamePlugin)
	return p, p.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from the servername directive of a
// server block.
func (s *serverNamePlugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	if !c.Next() {
		return c.ArgErr()
	}

	if !c.NextArg() {
		return c.ArgErr()
	}

	serverName := c.Val()

	if len(c.RemainingArgs()) > 0 || c.Next() {
		return c.ArgErr()
	}

	s.ServerName = serverName

	return nil
}

// Interface guards
var (
	_ caddyfile.Unmarshaler = (*serverNamePlugin)(nil)
	_ plugin.Middleware     = (*serverNamePlugin)(nil)
)
