package socket

import (
	"fmt"
	"net"
)

// Addr is a IPv4 address used to send directed unicasts (i.e. without
// a prior ARP request). The IP datagram will be sent to the provided MAC
// address without taking routing considerations into account!
type Addr struct {
	MAC  net.HardwareAddr
	IP   net.IP
	Port uint16 // FIXME: not yet used due to PreparePacket implementation

	Source struct {
		MAC  net.HardwareAddr
		IP   net.IP
		Port uint16 // FIXME: not yet used due to PreparePacket implementation
	}
}

// Network returns "raw" and implements net.Addr
func (a *Addr) Network() string {
	return "raw"
}

// String returns a string representation of the peer's address
func (a *Addr) String() string {
	return fmt.Sprintf("<%s>%s:%d", a.MAC.String(), a.IP.String(), a.Port)
}

// Compile time check
var _ net.Addr = &Addr{}