package lease

import (
	"context"
	"errors"
	"net"
	"time"
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
}

// Key is a key used to associate a Database with
// a context.Context
type Key struct{}

// GetDatabase returns the lease database assigned to ctx
func GetDatabase(ctx context.Context) Database {
	val := ctx.Value(Key{})
	if val == nil {
		return nil
	}

	db := val.(Database)
	return db
}

// WithDatabase returns a new context that has the given database
// assigned
func WithDatabase(ctx context.Context, db Database) context.Context {
	return context.WithValue(ctx, Key{}, db)
}
