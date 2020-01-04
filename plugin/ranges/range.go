package ranges

import (
	"context"
	"net"
	"time"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/iprange"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("range", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupRange,
	})
}

// RangePlugin assigned IP address from preconfigured ranges
type RangePlugin struct {
	// Next is the next handler in the chain
	Next plugin.Handler

	// Ranges holds all IP ranges that can be used by the plugin
	Ranges iprange.IPRanges

	// Network defines the network that is served by the plugin
	// setupRange copies this from the dhcpserver.Config
	Network net.IPNet

	// L holds the logger to use
	L log.Logger
}

func (p *RangePlugin) findUnboundAddr(ctx context.Context, mac net.HardwareAddr, requested net.IP, db lease.Database) net.IP {
	cli := lease.Client{
		HwAddr: mac,
		ID:     mac.String(),
	}

	// if there's a requested IP address will try that first if it's part of our range
	// if there's a requested address that's not in our ranges will do nothing as another
	// middleware might handle the request
	if requested != nil && !requested.IsUnspecified() {
		if !p.Ranges.Contains(requested) {
			// we cannot serve the requested IP address
			// may another middleware can
			p.L.Warnf("%s requsted %s which is not in our defined range", mac, requested)
			return nil
		}

		err := db.Reserve(ctx, requested, cli)
		if err == nil {
			p.L.Debugf("%s requested previous IP address %s", mac, requested)
			return requested
		}
		p.L.Warnf("%s requested previous IP address %s but we failed to reserve it: %s", mac, requested, err.Error())

		// TODO(ppacher): should we check for context errors here?
	}

	for _, r := range p.Ranges {
		for idx := 0; idx < r.Len(); idx++ {
			ip := r.ByIdx(idx)

			if err := db.Reserve(ctx, ip, cli); err != nil {
				// failed to reserve the IP address
				if err == context.DeadlineExceeded || err == context.Canceled {
					return nil
				}

				continue
			}

			// we successfully reserved the IP address for the client
			return ip
		}
	}

	// we failed to find a leasable address
	return nil
}

func (p *RangePlugin) findAndPrepareResponse(ctx context.Context, req, res *dhcpv4.DHCPv4, requested net.IP, db lease.Database) bool {
	ip := p.findUnboundAddr(ctx, req.ClientHWAddr, requested, db)
	if ip != nil {
		p.L.Debugf("found unbound address for %s: %s", req.ClientHWAddr, ip)
		res.YourIPAddr = ip

		// TODO(ppacher): should we move that to the dhcpserver.Server and make sure to always configure
		// the subnet mask? Check the static plugin as well
		if req.IsOptionRequested(dhcpv4.OptionSubnetMask) {
			res.UpdateOption(dhcpv4.OptSubnetMask(p.Network.Mask))
		}
		return true
	}
	return false
}

// ServeDHCP implements the plugin.Handler interface and served DHCP requests
func (p *RangePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	db := lease.GetDatabase(ctx)
	cli := lease.Client{HwAddr: req.ClientHWAddr}

	if dhcpserver.Discover(req) {
		if p.findAndPrepareResponse(ctx, req, res, req.RequestedIPAddress(), db) {
			return nil
		}
		p.L.Debugf("failed to find address for %s", req.ClientHWAddr)

		// Since we are the last plugin in the middleware chain we should do
		// our best to find an IP address for that client. That means ingoring the
		// requested IP address and trying again (it might be the prefered one from
		// a different network the client was previously attached to)

		// TODO(ppacher): should we try to call though the plugin-chain before trying this?
		// Is it really safe to assume we are the last one?

		if req.RequestedIPAddress() != nil && !req.RequestedIPAddress().IsUnspecified() {
			if p.findAndPrepareResponse(ctx, req, res, nil, db) {
				return nil
			}
		}

	} else

	// for DHCPREQUEST we try to actually lease the IP address
	// and send a DHCPACK if we succeeded. In any error case
	// we will NOT send a NAK as a middleware below us
	// may succeed in leasing the address
	// TODO(ppacher): we could check if the RequestedIPAddress() is inside
	// the IP ranges and then decide to ACK or NAK
	if dhcpserver.Request(req) {
		state := "binding"
		ip := req.RequestedIPAddress()
		if ip == nil || ip.IsUnspecified() {
			ip = req.ClientIPAddr
			state = "renewing"
		}

		if ip != nil && !ip.IsUnspecified() {
			p.L.Debugf("%s (%s) requests %s", req.ClientHWAddr, state, ip)

			// use the leaseTime already set to the response packet
			// else we fallback to time.Hour
			// TODO(ppacher): we should make the default lease time configurable
			// for the ranges plguin
			leaseTime := res.IPAddressLeaseTime(time.Hour)

			leaseTime, err := db.Lease(ctx, ip, cli, leaseTime, state == "renewing")
			if err == nil {
				p.L.Infof("%s (%s): lease %s for %s", req.ClientHWAddr, state, ip, leaseTime)
				if leaseTime == time.Hour {
					// if we use the default, make sure to set it
					res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
				}

				// make sure we ACK the DHCPREQUEST
				res.YourIPAddr = ip

				if res.SubnetMask() == nil || res.SubnetMask().String() == "0.0.0.0" {
					res.UpdateOption(dhcpv4.OptSubnetMask(p.Network.Mask))
				}

				res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))

				return nil
			}
			p.L.Errorf("%s: failed to lease requested ip %s: %s", req.ClientHWAddr, ip, err.Error())
		}
	} else

	// If it's a DHCPRELEASE message and part of our range we'll release it
	if dhcpserver.Release(req) && p.Ranges.Contains(req.ClientIPAddr) {
		if err := db.Release(ctx, req.ClientIPAddr); err != nil {
			return err
		}

		// No response should be sent for DHCPRELEASE messages
		return dhcpserver.ErrNoResponse
	}

	return p.Next.ServeDHCP(ctx, req, res)
}

// Name returns "range" and implements the plugin.Handler interface
func (p *RangePlugin) Name() string {
	return "range"
}

func setupRange(c *caddy.Controller) error {
	cfg := dhcpserver.GetConfig(c)
	plg := &RangePlugin{
		Network: cfg.Network,
	}
	plg.L = log.GetLogger(c, plg)

	for c.Next() {
		if !c.NextArg() {
			return c.ArgErr()
		}

		startIP := net.ParseIP(c.Val())
		if startIP == nil {
			return c.SyntaxErr("IPv4 address")
		}

		if !c.NextArg() {
			return c.ArgErr()
		}

		endIP := net.ParseIP(c.Val())
		if endIP == nil {
			return c.SyntaxErr("IPv4 address")
		}

		r := &iprange.IPRange{
			Start: startIP,
			End:   endIP,
		}

		plg.Ranges = iprange.Merge(append(plg.Ranges, r))
	}

	plg.L.Debugf("serving %d IP ranges: %v", len(plg.Ranges), plg.Ranges)

	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.Next = next
		return plg
	})

	return nil
}
