package lease

import (
	"fmt"
	"net"
	"time"
)

// Lease describes an IPv4 address that has been leased to a client
type Lease struct {
	// Client is the client that received the lease
	Client

	// Expires holds the timestamp in seconds when the lease is going to
	// expire
	Expires time.Time

	// Address holds the address that has been leased to the client
	Address net.IP
}

// ExpiredAt returns true if the lease was or will be expired at t
func (l *Lease) ExpiredAt(t time.Time) bool {
	return t.After(l.Expires)
}

// Expired returns true if the lease has already been expired
func (l *Lease) Expired() bool {
	return l.ExpiredAt(time.Now())
}

// String implements fmt.Stringer
func (l *Lease) String() string {
	suffix := ""
	if l.Expired() {
		suffix = "expired"
	} else {
		suffix = fmt.Sprintf("expires in %s", l.Expires.Sub(time.Now()))
	}
	return fmt.Sprintf("%s (%s; %s)", l.Address.String(), l.HwAddr, suffix)
}

// Clone returns a deep copy of the lease
func (l *Lease) Clone() *Lease {
	return &Lease{
		Client: Client{
			HwAddr:   append(net.HardwareAddr{}, l.Client.HwAddr...),
			ID:       l.Client.ID,
			Hostname: l.Client.Hostname,
		},
		Expires: l.Expires,
		Address: append(net.IP{}, l.Address...),
	}
}
