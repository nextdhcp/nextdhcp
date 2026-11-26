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

	// ID identifies the client, see ClientID
	ID ClientID
}

// ClientID identifies a client. Lease databases identify clients by their
// hardware address, in the form that net.HardwareAddr.String returns.
type ClientID string

// HardwareAddrID returns the ID of the client with the hardware address mac.
func HardwareAddrID(mac net.HardwareAddr) ClientID {
	return ClientID(mac.String())
}

// NewClient returns the client with the hardware address mac, which lease
// databases identify clients by, and which is also its ID.
func NewClient(mac net.HardwareAddr) Client {
	return Client{HwAddr: mac, ID: HardwareAddrID(mac)}
}

// String implements fmt.Stringer
func (cli *Client) String() string {
	return fmt.Sprintf("%s (hostname=%s, id=%s)", cli.HwAddr, cli.Hostname, cli.ID)
}

// Clone returns a deep copy of the client
func (cli *Client) Clone() Client {
	return Client{
		HwAddr:   append(net.HardwareAddr{}, cli.HwAddr...),
		Hostname: cli.Hostname,
		ID:       cli.ID,
	}
}
