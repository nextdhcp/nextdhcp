package lease

import (
	"fmt"
	"net"
)

// Client is a DHCP client
type Client struct {
	// HwAddr is the hardware address of the client for which IP has been reserved
	HwAddr net.HardwareAddr

	// Hostname may hold the hostname as reported by the client
	Hostname string

	// ID is the identifier used by the client
	ID string
}

// String implements fmt.Stringer
func (cli *Client) String() string {
	return fmt.Sprintf("%s (hostname=%s, id=%s)", cli.HwAddr, cli.Hostname, cli.ID)
}
