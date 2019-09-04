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

type rangePlugin struct {
	next    plugin.Handler
	ranges  iprange.IPRanges
	network net.IPNet
	l       log.Logger
}

func (p *rangePlugin) findUnboundAddr(ctx context.Context, mac net.HardwareAddr, requested net.IP, db lease.Database) net.IP {
	cli := lease.Client{
		HwAddr: mac,
		ID:     mac.String(),
	}

	// if there's a requested IP address will try that first if it's part of our range
	// if there's a requested address that's not in our ranges will do nothing as another
	// middleware might handle the request
	if requested != nil && !requested.IsUnspecified() {
		p.l.Debugf("%s requsted %s", mac, requested)
		if !p.ranges.Contains(requested) {
			// we cannot serve the requested IP address
			// may another middleware can
			p.l.Debugf("we cannot serve that one ...")
			return nil
		}

		if err := db.Reserve(ctx, requested, cli); err == nil {
			return requested
		}

		// TODO(ppacher): should we check for context errors here?
	}

	for _, r := range p.ranges {
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

func (p *rangePlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	db := lease.GetDatabase(ctx)
	cli := lease.Client{HwAddr: req.ClientHWAddr}

	if dhcpserver.Discover(req) {
		ip := p.findUnboundAddr(ctx, req.ClientHWAddr, req.RequestedIPAddress(), db)
		if ip != nil {
			p.l.Debugf("found unbound address for %s: %s", req.ClientHWAddr, ip)
			res.YourIPAddr = ip
			return nil
		}
		p.l.Debugf("failed to find address for %s", req.ClientHWAddr)
		// we failed to find an IP address for that client
		// so fallthrough and call the next middleware
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
			p.l.Debugf("%s (%s) requests %s", req.ClientHWAddr, state, ip)

			// use the leaseTime already set to the response packet
			// else we fallback to time.Hour
			// TODO(ppacher): we should make the default lease time configurable
			// for the ranges plguin
			leaseTime := res.IPAddressLeaseTime(time.Hour)

			leaseTime, err := db.Lease(ctx, ip, cli, leaseTime, state == "renewing")
			if err == nil {
				p.l.Infof("%s (%s): lease %s for %s", req.ClientHWAddr, state, ip, leaseTime)
				if leaseTime == time.Hour {
					// if we use the default, make sure to set it
					res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
				}

				// make sure we ACK the DHCPREQUEST
				res.YourIPAddr = ip

				if res.SubnetMask() == nil || res.SubnetMask().String() == "0.0.0.0" {
					res.UpdateOption(dhcpv4.OptSubnetMask(p.network.Mask))
				}

				res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))

				return nil
			}
			p.l.Errorf("%s: failed to lease requested ip %s: %s", req.ClientHWAddr, ip, err.Error())
		}
	} else

	// If it's a DHCPRELEASE message and part of our range we'll release it
	if dhcpserver.Release(req) && p.ranges.Contains(req.ClientIPAddr) {
		if err := db.Release(ctx, req.ClientIPAddr); err != nil {
			return err
		}

		// No response should be sent for DHCPRELEASE messages
		return dhcpserver.ErrNoResponse
	}

	return p.next.ServeDHCP(ctx, req, res)
}

func (p *rangePlugin) Name() string {
	return "ranges"
}

func setupRange(c *caddy.Controller) error {
	cfg := dhcpserver.GetConfig(c)
	plg := &rangePlugin{
		network: cfg.Network,
	}
	plg.l = log.GetLogger(c, plg)

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

		plg.ranges = iprange.Merge(append(plg.ranges, r))
	}

	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.next = next
		return plg
	})

	return nil
}
