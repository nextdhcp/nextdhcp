package test

import (
	"context"
	"errors"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

type (
	// HandlerFunc implements plugin.Handler
	HandlerFunc func(ctx context.Context, req, res *dhcpv4.DHCPv4) error
)

// ServeDHCP implements plugin.Handler
func (fn HandlerFunc) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	return fn(ctx, req, res)
}

// Name implements plugin.Handler
func (fn HandlerFunc) Name() string {
	return "test.HandlerFunc"
}

var (
	// ErrorHandler is a plugin.Handler and always returns an error
	ErrorHandler = HandlerFunc(func(_ context.Context, req, res *dhcpv4.DHCPv4) error {
		return errors.New("simulated error")
	})

	// NoOpHandler is a No-Operation plugin.Handler
	NoOpHandler = HandlerFunc(func(_ context.Context, req, res *dhcpv4.DHCPv4) error {
		return nil
	})
)
