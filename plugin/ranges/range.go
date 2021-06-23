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
	l := log.With(ctx, p.L)

	cli := lease.Client{
		HwAddr: mac,
		ID:     mac.String(),
	}

	// if there's a requested IP address will try that first if it's part of our range
	// if there's a requested address that's not in our ranges will do nothing as another
	// middleware might handle the request
	if ipIsSet(requested) {
		if !p.Ranges.Contains(requested) {
			// we cannot serve the requested IP address
			// may another middleware can
			l.Warnf("%s requsted %s which is not in our defined range", mac, requested)
			return nil
		}

		err := db.Reserve(ctx, requested, cli)
		if err == nil {
			l.Debugf("%s requested previous IP address %s", mac, requested)
			return requested
		}
		l.Warnf("%s requested previous IP address %s but we failed to reserve it: %s", mac, requested, err.Error())

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
		log.With(ctx, p.L).Debugf("found unbound address for %s: %s", req.ClientHWAddr, ip)
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
	l := log.With(ctx, p.L)
	// dhcp request has already been handled
	if dhcpserver.Ack(res) || dhcpserver.Nak(res) || dhcpserver.Offer(res) {
		l.Debugf("request already been handled %s, %s", req.ClientHWAddr, req.ClientIPAddr.String())
		return p.Next.ServeDHCP(ctx, req, res)
	}

	db := lease.GetDatabase(ctx)
	cli := lease.Client{HwAddr: req.ClientHWAddr}

	if dhcpserver.Discover(req) {
		if p.findAndPrepareResponse(ctx, req, res, req.RequestedIPAddress(), db) {
			return nil
		}
		l.Debugf("failed to find address for %s", req.ClientHWAddr)

		// Since we are the last plugin in the middleware chain we should do
		// our best to find an IP address for that client. That means ingoring the
		// requested IP address and trying again (it might be the prefered one from
		// a different network the client was previously attached to)

		// TODO(ppacher): should we try to call though the plugin-chain before trying this?
		// Is it really safe to assume we are the last one?

		if ipIsSet(req.RequestedIPAddress()) {
			if p.findAndPrepareResponse(ctx, req, res, nil, db) {
				return nil
			}
		}
		res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	} else

	// for DHCPREQUEST we try to actually lease the IP address
	// and send a DHCPACK if we succeeded. In any error case
	// we will NOT send a NAK as a middleware below us
	// may succeed in leasing the address

	if dhcpserver.Request(req) {
		state := "binding"
		ip := req.RequestedIPAddress()

		if ipIsSet(req.ClientIPAddr) {
			if ipIsUnset(ip) || req.ClientIPAddr.Equal(ip) {
				ip = req.ClientIPAddr
				state = "renewing"
			}
		}

		if ipIsSet(ip) {
			l.Debugf("%s (%s) requests %s", req.ClientHWAddr, state, ip)

			if !p.Ranges.Contains(ip) {
				l.Infof("Ignoring lease request for %s: requested IP not inside the configured ranges %s", ip, p.Ranges.String())
				// fallthrough to the reset of the handler chain.
				// If no-one is able to lease the requested IP the server will respond with
				// DHCPNAK anyway.
				return p.Next.ServeDHCP(ctx, req, res)
			}

			// use the leaseTime already set to the response packet
			// else we fallback to time.Hour
			// TODO(ppacher): we should make the default lease time configurable
			// for the ranges plguin
			activeLeaseTime := res.IPAddressLeaseTime(time.Hour)
			renewLeaseTime := state == "renewing"

			leaseTime, err := db.Lease(ctx, ip, cli, activeLeaseTime, renewLeaseTime)

			if err == nil {
				l.Infof("%s (%s): lease %s for %s (activeLeaseTime: %s)", req.ClientHWAddr, state, ip, leaseTime, activeLeaseTime)
				if leaseTime == time.Hour {
					res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))
				}

				// make sure we ACK the DHCPREQUEST
				res.YourIPAddr = ip

				p.maySetSubnetMask(req, res)

				res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))

				return nil
			}

			l.Errorf("%s: failed to lease requested ip %s: %s", req.ClientHWAddr, ip, err.Error())
			p.logAddressReservedError(ctx, err, db, ip, req)
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

func (p *RangePlugin) maySetSubnetMask(req, res *dhcpv4.DHCPv4) {
	if !req.IsOptionRequested(dhcpv4.OptionSubnetMask) {
		return
	}

	if res.SubnetMask() == nil || res.SubnetMask().String() == "0.0.0.0" {
		res.UpdateOption(dhcpv4.OptSubnetMask(p.Network.Mask))
	}
}

func (p *RangePlugin) logAddressReservedError(ctx context.Context, err error, db lease.Database, ip net.IP, req *dhcpv4.DHCPv4) {
	l := log.With(ctx, p.L)
	if err != lease.ErrAddressReserved {
		return
	}

	reservedAddresses, raErr := db.ReservedAddresses(ctx)
	if raErr == nil {
		entry := reservedAddresses.FindIP(ip)
		if entry == nil {
			l.Errorf("%s: Database.Lease failed but IP %s is not reserved", req.ClientHWAddr, ip)
		} else {
			l.Errorf("%s: IP %s is already reserved for %s and expires %s (expired=%v)", req.ClientHWAddr, ip, entry.Client, entry.Expires, entry.Expired(time.Now()))
		}
	} else {
		l.Debugf("%s: failed to get list of reserved addresses: %s", req.ClientHWAddr, raErr)
	}
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

func ipIsUnset(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified()
}

func ipIsSet(ip net.IP) bool {
	return ip != nil && !ip.IsUnspecified()
}
