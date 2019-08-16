package middleware

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease"
)

// Context is passed to middlewares and provides additional information and helper
// methods for handling DHCPv4 requests
type Context struct {
	// Resp is the prepared DHCPv4 response. If you need to replace it manually use Replace()
	Resp *dhcpv4.DHCPv4

	ctx        context.Context  // embedded request context for cancellation
	req        *dhcpv4.DHCPv4   // holds the request served
	shouldSkip bool             // set to true if we should skip the entire request
	peer       net.Addr         // peer is the peer that send the request
	peerHwAddr net.HardwareAddr // the hardware (MAC) address of the peer
	iface      net.Interface    // the interface that received the request
	db         lease.Database   // the lease database assigned to the subnet/interface
}

// Peer returns the peer that sent the request. Note that for DHCPv4 requests sent by clients
// in INIT or INIT-REBOOT state the peer might be set to the UDP broadcast address
func (c *Context) Peer() net.Addr {
	return c.peer
}

// PeerHardwareAddr returns the hardware address of the peer as stated in the ethernet header
// frames. Note that this address may differ from the ClientHwAddr reported in the DHCPv4 request
// message in case of DHCP relay agents and routed unicast messages
func (c *Context) PeerHardwareAddr() net.HardwareAddr {
	return c.peerHwAddr
}

// Interface returns the incoming interface if known
func (c *Context) Interface() net.Interface {
	return c.iface
}

// InterfaceName is the name of the interface the DHCPv4 request has been received.
// If not known an empty string is returned
func (c *Context) InterfaceName() string {
	return c.iface.Name
}

// Database may return a lease.Database that has been assigned to the interface the request
// arrived on
func (c *Context) Database() lease.Database {
	return c.db
}

// WithOption sets a new option for the DHCPv4 response
func (c *Context) WithOption(opt dhcpv4.Option) {
	c.Resp.UpdateOption(opt)
}

// HasOption checks if the response has a given option already set
func (c *Context) HasOption(opt dhcpv4.Option) bool {
	return c.HasOptionCode(opt.Code.Code())
}

// HasOptionCode returns true if an option with the given code is
// already set on the response
func (c *Context) HasOptionCode(code uint8) bool {
	_, ok := c.Resp.Options[code]
	return ok
}

// Replace replaces the current response message with a new one
func (c *Context) Replace(msgType dhcpv4.MessageType, modifiers ...dhcpv4.Modifier) error {
	newResp, err := dhcpv4.NewReplyFromRequest(c.req, dhcpv4.PrependModifiers(modifiers, dhcpv4.WithMessageType(msgType))...)
	if err != nil {
		return err
	}

	c.Resp = newResp
	return nil
}

// SkipRequest aborts the middleware chain and completly skips serving the DHCP
// request
func (c *Context) SkipRequest() {
	c.shouldSkip = true
}
