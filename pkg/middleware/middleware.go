package middleware

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Context is passed to handler functions and carries additional information about the request
type Context interface {
	// Context returns the wrapped request context
	Context() context.Context
}

// Handler is a DHCPv4 request handler. The request is passed down the handler chain until a
// middleware handler returns true
type Handler func(ctx Context, request, response *dhcpv4.DHCPv4)
