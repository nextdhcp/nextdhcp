package config

import "net"

// PluginConfig configures an external plugin
type PluginConfig map[string]interface{}

// Name returns the name of the plugin
func (p PluginConfig) Name() (string, bool) {
	n, ok := p["name"]
	if !ok {
		return "", false
	}

	s, ok := n.(string)
	return s, ok
}

// Path returns the path to the plugin file
func (p PluginConfig) Path() (string, bool) {
	path, ok := p["path"]
	if !ok {
		return "", false
	}

	s, ok := path.(string)
	return s, ok
}

// SubnetConfig holds configuration values for a subnet served by dhcp-ng
type SubnetConfig struct {
	// ListenIP Is the IP address that should be listened on. It is extracted
	// from the map key
	ListenIP net.IP `json:"-"`

	// Network is the IP network that is served by this subnet
	Network *net.IPNet

	// Interface to listen on. If omitted, the interface is determined by the subnet
	// IP address
	Interface string `json:"interface,omitempty"`

	// ServerID is the server ID to use. If empty the IP address of the incoming network
	// interface will be used
	ServerID string `json:"serverIdentifier,omitempty"`

	// Ranges is a list of IP ranges that can be used for address
	// leases
	Ranges []string `json:"ranges,omitempty"`

	// Driver specifies the database driver to use.
	Driver string `json:"driver,omitempty"`

	// Options defines additional DHCP options for clients
	Options map[string]interface{} `json:"options,omitempty"`

	// LeaseTime configures the lease time for clients
	LeaseTime string `json:"leaseTime,omitempty"`

	// Handlers holds a list of middleware handlers that should
	// be used
	Handlers []string `json:"handlers,omitempty"`
}

// Config describes the configuration structure for dhcp-ng
type Config struct {
	// Plugins holds a list of plugin configurations
	Plugins []PluginConfig `json:"plugins,omitempty"`

	// Subnets holds all configured subnet declarations
	Subnets map[string]*SubnetConfig `json:"subnets,omitempty"`
}

// Prepare pre
func (c *Config) Prepare() error {
	// parse the subnet CIDR map key and add it to the struct
	for key, cfg := range c.Subnets {
		ip, ipNet, err := net.ParseCIDR(key)
		if err != nil {
			return err
		}

		cfg.ListenIP = ip
		cfg.Network = ipNet
	}

	return nil
}
