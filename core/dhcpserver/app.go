package dhcpserver

import (
	"errors"
	"fmt"

	"github.com/caddyserver/caddy/v2"
)

func init() {
	caddy.RegisterModule(App{})
}

// App is the "dhcp" Caddy app. It serves DHCPv4 clients on one or more
// subnets.
type App struct {
	// Servers are the subnets to serve, one for each server block of the
	// Dhcpfile.
	Servers []*Server `json:"servers,omitempty"`

	failed chan error
}

// CaddyModule returns the Caddy module information.
func (App) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp",
		New: func() caddy.Module { return new(App) },
	}
}

// Provision provisions all servers.
func (a *App) Provision(ctx caddy.Context) error {
	a.failed = make(chan error, 1)

	for _, s := range a.Servers {
		s.failed = a.failed

		if err := s.Provision(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Failed returns a channel that receives an error when a server stops
// serving because reading requests fails, for example because its network
// interface went away. Servers don't recover from that, so NextDHCP exits
// then, and a service manager can restart it. Call Failed after Provision.
func (a *App) Failed() <-chan error {
	return a.failed
}

// Start starts serving all subnets.
func (a *App) Start() error {
	for i, s := range a.Servers {
		if err := s.start(); err != nil {
			// Caddy doesn't stop an app that failed to start
			for _, started := range a.Servers[:i] {
				_ = started.stop()
			}

			return err
		}
	}

	// Only remove the leases that contradict static reservations once all
	// servers serve, so they are kept if NextDHCP fails to start
	for _, s := range a.Servers {
		s.removeStaticConflicts()
	}

	if info := getStartupInfo(a.Servers); info != "" {
		// Print not Println because info contains a trailing new line
		fmt.Print(info)
	}

	return nil
}

// Stop stops serving all subnets.
func (a *App) Stop() error {
	var errs []error
	for _, s := range a.Servers {
		if err := s.stop(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Cleanup closes the lease databases of all servers.
func (a *App) Cleanup() error {
	var errs []error
	for _, s := range a.Servers {
		if err := s.Cleanup(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Interface guards
var (
	_ caddy.App          = (*App)(nil)
	_ caddy.Provisioner  = (*App)(nil)
	_ caddy.CleanerUpper = (*App)(nil)
)
