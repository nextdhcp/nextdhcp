package utils

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/handler"
	"github.com/ppacher/dhcp-ng/pkg/lease"
	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
	"github.com/ppacher/dhcp-ng/pkg/lua"
	"github.com/ppacher/dhcp-ng/pkg/middleware"
)

var (
	// ErrInvalidSubnetRange is returned if a configured  IP range is invalid
	ErrInvalidSubnetRange = errors.New("invalid subnet range")
)

func getIPRanges(rangeDefinition [][]string) (iprange.IPRanges, error) {
	var ranges iprange.IPRanges

	// for all configured IP ranges, add them to the database
	for _, r := range rangeDefinition {
		if len(r) != 2 {
			return nil, ErrInvalidSubnetRange
		}

		startIP := net.ParseIP(r[0])
		endIP := net.ParseIP(r[1])

		if startIP == nil || endIP == nil {
			return nil, ErrInvalidSubnetRange
		}

		ipRange := &iprange.IPRange{startIP, endIP}
		if err := ipRange.Validate(); err != nil {
			return nil, err
		}

		ranges = append(ranges, ipRange)
	}

	return ranges, nil
}

// SubnetConfigFromLua converts a lua based subnet configuration to the struct required
// by `ppacher/dhcpv-ng/pkg/handler`
func SubnetConfigFromLua(runner *lua.Runner, subnet lua.Subnet) (*handler.SubnetConfig, error) {
	h := &handler.SubnetConfig{}

	ip, iface, err := FindInterface(subnet.IP.String())
	if err != nil {
		return nil, err
	}

	if ip.String() != subnet.IP.String() {
		panic("wrong universe detected")
	}

	h.IP = ip
	h.Interface = *iface

	h.Options = make(map[dhcpv4.OptionCode]dhcpv4.OptionValue, len(subnet.Options))
	// convert DHCP options
	for key, value := range subnet.Options {
		// TODO(ppacher): fix option conversion
		_ = key
		_ = value

		return nil, errors.New("DHCP options not yet implemented")
	}

	leaseTime, err := time.ParseDuration(subnet.LeaseTime)
	if err != nil {
		return nil, err
	}

	h.LeaseTime = leaseTime

	// convert the string definition of IP ranges to iprange.IPRanges
	ranges, err := getIPRanges(subnet.Ranges)
	if err != nil {
		return nil, err
	}

	h.Network = subnet.Network

	options := make(map[string]interface{}, len(subnet.DatabaseOptions))
	for key, value := range subnet.DatabaseOptions {
		options[key] = value
	}

	if _, ok := options["network"]; !ok {
		options["network"] = subnet.Network
	}

	// try to open the database
	db, err := lease.Open(subnet.Database, options)
	if err != nil {
		return nil, err
	}

	// configure allowed IP ranges
	if err := db.AddRange(ranges...); err != nil {
		return nil, err
	}

	h.Database = db

	if subnet.Offer != nil {
		offerHandler := runner.FunctionHandler(subnet.Offer)

		// subnet.Offer should only be called for DHCPDISCOVER requests (and if we are going to send an offer)
		// so wrap the middleware and filter out other request message types
		h.Middlewares = append(h.Middlewares, middleware.HandleFunc(
			func(ctx *middleware.Context, req *dhcpv4.DHCPv4) {
				if req.MessageType() == dhcpv4.MessageTypeDiscover {
					offerHandler.Serve(ctx, req)
				}
			},
		))
	}

	return h, nil
}

// FindInterface parses ipStr and return the interface the IP is bound on
func FindInterface(ipStr string) (net.IP, *net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		ip, _, err = net.ParseCIDR(ipStr)
		if err != nil {
			return nil, nil, err
		}
	}

	if ip == nil {
		return nil, nil, fmt.Errorf("invalid IP address or CIDR notation: %s", ipStr)
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, nil, err
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.Equal(ip) {
				return ip, &iface, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("failed to find interface for %s", ipStr)
}
