package static

import (
	"errors"
	"fmt"
	"net"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(Plugin{})
	dhcpserver.RegisterHandlerDirective("static", setupStatic)
}

// CaddyModule returns the Caddy module information.
func (Plugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.static",
		New: func() caddy.Module { return new(Plugin) },
	}
}

// Provision sets up the plugin for the server it is provisioned by.
func (s *Plugin) Provision(ctx caddy.Context) error {
	s.Config = dhcpserver.ServerFromContext(ctx)
	if s.Config == nil {
		return errors.New("static must be provisioned by a DHCP server")
	}

	db := s.Config.Database
	if db == nil {
		return errors.New("static requires the lease database of the DHCP server")
	}

	s.L = ctx.Logger().Sugar()

	// ServeDHCP looks clients up by the canonical (lower case) form of their
	// MAC address, so store the addresses by that form
	addresses := make(map[string]net.IP, len(s.Addresses))
	macs := make(map[string]net.HardwareAddr, len(s.Addresses))
	clients := make(map[string]string, len(s.Addresses))
	for key, ip := range s.Addresses {
		mac, err := net.ParseMAC(key)
		if err != nil {
			return fmt.Errorf("invalid MAC address %s: %w", key, err)
		}

		if ip == nil {
			return fmt.Errorf("no IP address configured for client %s", key)
		}

		if _, ok := addresses[mac.String()]; ok {
			return fmt.Errorf("static IP address has already been configured for client %s", mac)
		}

		if other, ok := clients[ip.String()]; ok {
			return fmt.Errorf("IP %s already used for client %s", ip, other)
		}

		addresses[mac.String()] = ip
		macs[mac.String()] = mac
		clients[ip.String()] = key
	}

	s.Addresses = addresses

	// Reserve the addresses in the lease database, so other plugins, like
	// range, don't lease them to other clients
	for key, ip := range addresses {
		cli := lease.Client{HwAddr: macs[key], ID: key}
		if err := db.ReserveStatic(ctx, ip, cli); err != nil {
			return fmt.Errorf("failed to reserve static IP %s for client %s: %w", ip, key, err)
		}
	}

	return nil
}

// Wrap implements plugin.Middleware.
func (s *Plugin) Wrap(next plugin.Handler) plugin.Handler {
	s.Next = next
	return s
}

func setupStatic(h dhcpserver.Helper) (caddy.Module, error) {
	plg := new(Plugin)
	return plg, plg.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all static directives of a
// server block.
func (s *Plugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	addr := make(map[string]net.IP)
	ips := make(map[string]struct{})

	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		mac, err := net.ParseMAC(c.Val())
		if err != nil {
			return c.ArgErr()
		}
		key := mac.String()

		if !c.NextArg() {
			return c.ArgErr()
		}
		ip := net.ParseIP(c.Val())
		if ip == nil {
			return c.ArgErr()
		}

		if e, ok := addr[key]; ok {
			return fmt.Errorf("static IP address %s has already been configured for client %s", e.String(), key)
		}

		if _, ok := ips[ip.String()]; ok {
			return fmt.Errorf("IP %s already used for client %s", ip, key)
		}

		addr[key] = ip
		ips[ip.String()] = struct{}{}
	}

	s.Addresses = addr

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*Plugin)(nil)
	_ caddyfile.Unmarshaler = (*Plugin)(nil)
	_ plugin.Middleware     = (*Plugin)(nil)
)
