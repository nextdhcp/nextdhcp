package config

// PluginConfig configures an external plugin
type PluginConfig map[string]interface{}

// SubnetConfig holds configuration values for a subnet served by dhcp-ng
type SubnetConfig struct {
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
	Subnets map[string]SubnetConfig `json:"subnets,omitempty"`
}
