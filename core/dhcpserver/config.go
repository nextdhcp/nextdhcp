package dhcpserver

import (
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
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
	
	Ranges iprange.IPRanges

	// Database is the lease database that is queried for new leases and reservations
	//Database lease.Database

	// Options holds a map of DHCP options that should be set
	Options map[dhcpv4.OptionCode]dhcpv4.OptionValue

	// LeaseTime is the default lease time to use for new IP address leases
	LeaseTime time.Duration

	// Middlewares is the middleware stack to execute. See documentation of the DHCPv4
	// interface for more information
	//Middlewares []middleware.Handler
}
