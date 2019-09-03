package plugin

import (
	"context"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

type (
	// Handler for DHCP requests created by a plugin factory (see Plugin).
	// Each handler is responsible of calling the next handling in the chain
	// which was passed to Plugin 
	Handler interface {
		// Name returns the name of the handler
		Name() string
		
		// ServeDHCP is a HandlerFunc and called for each DHCPv4 request. See HandlerFunc
		// for more information
		ServeDHCP(ctx context.Context, req *dhcpv4.DHCPv4, resp *dhcpv4.DHCPv4) error
	}
	
	// Plugin represents Setup func for a dhcp-ng plugin. It is passed the
	// next plugin in the chain
	Plugin func(Handler) Handler
	
	// HandlerFunc allows to easily wrap a function as a Handler type
	// The provided context will always have a lease.Database assigned to it. A 
	// reference to the database can be loaded by using lease.GetDatabase(ctx)
	HandlerFunc func(ctx context.Context, req *dhcpv4.DHCPv4, resp *dhcpv4.DHCPv4) error
)

// ServeDHCP implements the Handler interface
func (fn HandlerFunc) ServeDHCP(ctx context.Context, req, resp *dhcpv4.DHCPv4) error {
	return fn(ctx, req, resp)
}

// Name returns "HandlerFunc" and implements the Handler interface
func (fn HandlerFunc) Name() string {
	return "HandlerFunc"
}
