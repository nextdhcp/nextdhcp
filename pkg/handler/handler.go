package handler

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"

	"github.com/ppacher/dhcp-ng/pkg/lease"
	"github.com/ppacher/dhcp-ng/pkg/middleware"
)

// SubnetConfig holds settings required to serve DHCP requests on a given
// subnet
type SubnetConfig struct {
	// IP is the IP address of the interface we are listening on. This is required
	// to select the right subnet configuration when listening and serving multiple
	// subnets
	IP net.IP

	// Network is the network of the subnet
	Network net.IPNet

	// Interface is the network interface where the subnet should be served. This
	// is required to select the right subnet configuration when listening and serving
	// multiple subnets
	Interface net.Interface

	// Database is the lease database that is queried for new leases and reservations
	Database lease.Database

	// Options holds a map of DHCP options that should be set
	Options map[dhcpv4.OptionCode]dhcpv4.OptionValue

	// LeaseTime is the default lease time to use for new IP address leases
	LeaseTime time.Duration

	// Middlewares is the middleware stack to execute. See documentation of the DHCPv4
	// interface for more information
	Middlewares []middleware.Handler
}

// Option holds configuration options for DHCP handler
type Option struct {
	// Subnets to serve
	Subnets []SubnetConfig
}

// DHCPv4 handles incoming DHCPv4 messages. It prepares a reply package with configured
// options and a possible IP address. The reply message is then passed down a pre-configured
// middleware stack where each middleware can alter the request message. See `github.com/ppacher/dhcp-ng/pkg/middleware`
// for more information on middleware implementations
type DHCPv4 interface {
	Serve(context.Context, net.Interface, *net.UDPAddr, net.HardwareAddr, *dhcpv4.DHCPv4) *dhcpv4.DHCPv4
}

// NewV4 creates a new DHCPv4 handler
func NewV4(options Option) DHCPv4 {
	return &v4handler{
		subnets:                   options.Subnets,
		prepareDHCPv4Offer:        prepareDHCPv4Offer,
		prepareDHCPv4RequestReply: prepareDHCPv4RequestReply,
		handleDHCPv4Release:       handleDHCPv4Release,
	}
}

type serveFunc func(ctx *middleware.Context, req *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error)

type v4handler struct {
	subnets []SubnetConfig

	// function responsible to handle incoming DHCP request types
	// part of the struct for unit testing purposes
	prepareDHCPv4Offer        serveFunc
	prepareDHCPv4RequestReply serveFunc
	handleDHCPv4Release       serveFunc
}

func (d *v4handler) findSubnet(iface net.Interface) *SubnetConfig {
	for _, s := range d.subnets {
		if s.Interface.Name == iface.Name {
			return &s
		}
	}

	return nil
}

func (d *v4handler) Serve(ctx context.Context, iface net.Interface, peer *net.UDPAddr, hw net.HardwareAddr, req *dhcpv4.DHCPv4) *dhcpv4.DHCPv4 {
	log.Printf("got request on %s from %s (hw: %s) type %s", iface.Name, peer.String(), hw.String(), req.MessageType().String())

	s := d.findSubnet(iface)
	if s == nil {
		log.Println("failed to serve request: failed to find subnet configuration for ", iface.Name)
		return nil
	}

	serveCtx, err := middleware.NewContext(ctx, req, peer, hw, iface, nil)
	if err != nil {
		log.Println("failed to serve request: failed to create context: ", err.Error())
		return nil
	}

	resp, err := d.prepareResponse(serveCtx, req, s)
	if err != nil {
		log.Println("failed to serve request: ", err.Error())
		return nil
	}

	serveCtx.Resp = resp

	for _, handler := range s.Middlewares {
		handler.Serve(serveCtx, req)

		if serveCtx.ShouldSkip() {
			return nil
		}
	}

	if serveCtx.Resp == nil {
		log.Println("warning: no response message returned. use ctx.ShouldSkip() instead")
		return nil
	}

	log.Println("Response: \n", serveCtx.Resp.Summary())

	return serveCtx.Resp
}

func (d *v4handler) prepareResponse(ctx *middleware.Context, req *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error) {
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		return d.prepareDHCPv4Offer(ctx, req, s)
	case dhcpv4.MessageTypeRequest:
		return d.prepareDHCPv4RequestReply(ctx, req, s)
	case dhcpv4.MessageTypeRelease:
		return d.handleDHCPv4Release(ctx, req, s)
	case dhcpv4.MessageTypeDecline:
		return nil, fmt.Errorf("decline messages are denied")
	}

	return nil, fmt.Errorf("unsupported message type %s", req.MessageType().String())
}
