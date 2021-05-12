package dhcpserver

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/apex/log"
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/lease"
	dhcpLog "github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/plugin"
	"github.com/nextdhcp/nextdhcp/plugin/logger"
)

// Config configures a DHCP server subnet
type Config struct {
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

	// plugins is a list of middleware setup functions
	plugins []plugin.Plugin

	// last plugin has been set
	lastPlugin bool

	// chain is the beginning of the middleware chain for this subnet
	chain plugin.Handler

	// logger holds the logger instance for this subnet
	logger log.Interface
}

// AddPlugin adds a new plugin to the middleware chain
func (cfg *Config) AddPlugin(p plugin.Plugin) {
	cfg.logger.Debugf("registered plugin %#v", p)
	cfg.plugins = append(cfg.plugins, p)
}

func keyForConfig(serverBlockIndex int) string {
	return fmt.Sprintf("%d", serverBlockIndex)
}

// GetConfig gets the Config that corresponds to c
// if none exist nil is returned
func GetConfig(c *caddy.Controller) *Config {
	ctx := c.Context().(*dhcpContext)
	key := keyForConfig(c.ServerBlockIndex)

	cfg := ctx.keyToConfig[key]
	return cfg
}

func buildMiddlewareChain(cfg *Config) error {
	var endOfChainHandler plugin.HandlerFunc = func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
		peer := GetPeer(ctx)
		dhcpLog.With(ctx)

		// if it's a DHCPREQUEST that we didn't handle yet we will send
		// DHCPNAK
		if Request(req) {
			logger.Log.Warnf("unhandled DHCPREQUEST, responding with DHCPNAK")
			res.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
			return nil
		}

		logger.Log.Infof("%s from %s not handled. dropping", req.MessageType().String(), peer)
		return ErrNoResponse
	}

	var chain plugin.Handler = endOfChainHandler
	for i := len(cfg.plugins) - 1; i >= 0; i-- {
		chain = cfg.plugins[i](chain)
		logger.Log.Debugf("plugin (%d) %s setup", i, chain.Name())
	}

	cfg.chain = chain

	return nil
}
