package lease

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
)

var (
	// ErrAddressInUse indicates that the requested IP address is already bound to a different client
	ErrAddressInUse = errors.New("IP is already used by a different client")

	// ErrAddressReserved indicates that the requested IP address is already reserved for a different client
	ErrAddressReserved = errors.New("IP is already reserved for a different client")

	// ErrNoIPAvailable indicates that no usable IP address have been found
	ErrNoIPAvailable = errors.New("no IP address is available")

	// ErrInvalidAddress indicates that the IP address is invalid
	ErrInvalidAddress = errors.New("invalid IP address")
)

// Database describes a lease database interface
type Database interface {
	// Leases returns all registered and not-yet-released IP address
	// leases
	Leases(context.Context) ([]Lease, error)

	// ReservedAddresses returns a slice of currently reserved IP addresses
	// These addresses will not be used when search for available addresses
	ReservedAddresses(context.Context) ([]ReservedAddress, error)

	// FindAddress tries to find a free address for the given client. If the
	// client already has a leased IP address that address is returned
	FindAddress(context.Context, *Client) (net.IP, error)

	// Reserve tries to reserve the IP address for a client
	Reserve(context.Context, net.IP, Client) error

	// Lease an IP address for a client. The IP address must either already be leased to the
	// client or have been reserved for it
	Lease(context.Context, net.IP, Client, time.Duration, bool) (time.Duration, error)

	// Release releases a previous client IP address lease. If no such lease exists the list
	// of reserved IP addresses is checked and any reservation for the client is removed
	Release(context.Context, net.IP) error

	// DeleteReservation deletes a IP address reservation
	DeleteReservation(context.Context, net.IP, *Client) error

	// ReleaseClient releases all IP address leases or reservations for the given client
	ReleaseClient(context.Context, *Client) error

	// AddRange adds new ranges to the list of IP addresses that can be leased
	AddRange(ranges ...*iprange.IPRange) error

	// DeleteRange deletes ranges from the list of leasable IP addresses. Already leased addreses
	// will still be valid until they expire
	DeleteRange(ranges ...*iprange.IPRange) error
}
