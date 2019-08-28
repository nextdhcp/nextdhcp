package middleware

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Handler is a middleware that handles incoming DHCPv4 requests
type Handler interface {
	Serve(ctx *Context, request *dhcpv4.DHCPv4)
}

// HandleFunc is a middleware that handles incoming DHCPv4 requests
// Each HandleFunc automatically satisfies the Handler interface
type HandleFunc func(ctx *Context, request *dhcpv4.DHCPv4)

// Serve implements the Handler interface
func (h HandleFunc) Serve(ctx *Context, request *dhcpv4.DHCPv4) {
	h(ctx, request)
}
