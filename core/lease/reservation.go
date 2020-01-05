package lease

import (
	"net"
	"time"
)

// ReservedAddress represents an IPv4 address that has been reserved for a specific client.
// A reserved address may have an expiration time
type ReservedAddress struct {
	Client

	// IP is the IP that has been reserved
	IP net.IP

	// Expires holds the time the reservation expires. nil if the reservation
	// cannot expire (i.e. static leases from the IP range pool)
	Expires *time.Time
}

// Expired checks if the reserved address has been expired at time t
func (r ReservedAddress) Expired(t time.Time) bool {
	if r.Expires == nil {
		return false
	}

	return r.Expires.After(t)
}

// ReservedAddressList adds utility methods to a slice of ReservedAddress'es
type ReservedAddressList []ReservedAddress

// FindIP searches the list of reserved addresses for ip
func (rl ReservedAddressList) FindIP(ip net.IP) *ReservedAddress {
	for _, r := range rl {
		if r.IP.Equal(ip) {
			return &r
		}
	}

	return nil
}

// FindMAC searches the list of reserved addresses for mac
func (rl ReservedAddressList) FindMAC(mac net.HardwareAddr) *ReservedAddress {
	for _, r := range rl {
		if r.HwAddr.String() == mac.String() {
			return &r
		}
	}

	return nil
}

// FindHostname searches the list of reserved addresses for name
func (rl ReservedAddressList) FindHostname(name string) *ReservedAddress {
	for _, r := range rl {
		if r.Hostname == name {
			return &r
		}
	}

	return nil
}

// FindID searches the list of reserved addresses for ID
func (rl ReservedAddressList) FindID(id string) *ReservedAddress {
	for _, r := range rl {
		if r.ID == id {
			return &r
		}
	}

	return nil
}
