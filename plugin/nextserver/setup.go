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

	// The server address field (siaddr) of DHCPv4 messages only holds an
	// IPv4 address: serializing a response with another address panics
	ip := net.ParseIP(serverName)
	if ip.To4() == nil {
		return fmt.Errorf("%s:%d: expected IPv4 address but got %s", c.File(), c.Line(), serverName)
	}

	if len(c.RemainingArgs()) > 0 || c.Next() {
		return c.ArgErr()
	}

	s.NextServer = ip

	return nil
}

// Validate checks the next server of a JSON config like UnmarshalCaddyfile
// checks the one of a Dhcpfile. Without one, responses have no server
// address.
func (s *nextServerPlugin) Validate() error {
	if s.NextServer != nil && s.NextServer.To4() == nil {
		return fmt.Errorf("next server %s is not an IPv4 address", s.NextServer)
	}

	return nil
}

// Interface guards
var (
	_ caddy.Validator       = (*nextServerPlugin)(nil)
	_ caddyfile.Unmarshaler = (*nextServerPlugin)(nil)
	_ plugin.Middleware     = (*nextServerPlugin)(nil)
)
