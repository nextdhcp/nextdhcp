package lease

import "net"

type Client struct {
	// HwAddr is the hardware address of the client for which IP has been reserved
	HwAddr net.HardwareAddr

	// Hostname may hold the hostname as reported by the client
	Hostname string

	// ID is the identifier used by the client
	ID string
}
