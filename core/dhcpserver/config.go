package dhcpserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/plugin"
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

	// chain is the beginning of the middleware chain for this subnet
	chain plugin.Handler
}

// AddPlugin adds a new plugin to the middleware chain
func (cfg *Config) AddPlugin(p plugin.Plugin) {
	cfg.plugins = append(cfg.plugins, p)
}

func keyForConfig(serverBlockIndex, serverBlockKeyIndex int) string {
	return fmt.Sprintf("%d:%d", serverBlockIndex, serverBlockKeyIndex)
}

// GetConfig gets the Config that corresponds to c
// if none exist nil is returned
func GetConfig(c *caddy.Controller) *Config {
	ctx := c.Context().(*dhcpContext)
	key := keyForConfig(c.ServerBlockIndex, c.ServerBlockKeyIndex)

	cfg := ctx.keyToConfig[key]
	return cfg
}

func buildMiddlewareChain(cfg *Config) error {
	var endOfChainHandler plugin.HandlerFunc = func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
		peer := GetPeer(ctx)
		log.Printf("%s from %s not handled. dropping", req.MessageType().String(), peer)

		return ErrNoResponse
	}

	fmt.Println("building chain for ", cfg.plugins)

	var chain plugin.Handler = endOfChainHandler
	for i := len(cfg.plugins) - 1; i >= 0; i-- {
		chain = cfg.plugins[i](chain)
	}

	cfg.chain = chain

	return nil
}
