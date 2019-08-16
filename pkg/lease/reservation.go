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
