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

	// ErrStaticReservation indicates that the IP address is statically
	// reserved, which only the configuration can change
	ErrStaticReservation = errors.New("IP is statically reserved")
)

// Database describes a lease database interface
type Database interface {
	// Leases returns all registered and not-yet-released IP address
	// leases. IP leases that are already expired are returned as well
	Leases(context.Context) ([]Lease, error)

	// ReservedAddresses returns a slice of currently reserved IP
	// addresses. These addresses will not be used when search for
	// available addresses. Reservations that already expired are
	// returned as well
	ReservedAddresses(context.Context) (ReservedAddressList, error)

	// Reserve tries to reserve the IP address for a client
	// If the IP address is already reserved for a different client,
	// and those reservation has not expired an error is returned.
	// If theres no reservation for the IP (or it has expired),
	// a new reservation is stored. Note that it is not possible
	// to increase the expiration time of an existing,
	// not-yet-expired, reservation
	Reserve(context.Context, net.IP, Client) error

	// Lease an IP address for a client. The IP address must
	// either already be leased to the client or have been reserved
	// for it. if renew is set, the lease time will always be
	// updated. Otherwise, the lease time will only be updated if
	// the IP address was only reserved for the client, or if the
	// lease already expired. The returned lease time indicates
	// the active lease time either with or without renewal. Any
	// pending reservation for this IP address will be removed
	Lease(context.Context, net.IP, Client, time.Duration, bool) (time.Duration, error)

	// Release releases a previous client IP address lease. If no
	// such lease exists the list of reserved IP addresses is
	// checked and any reservation for the client is removed
	Release(context.Context, net.IP) error

	// DeleteReservation deletes a IP address reservation
	DeleteReservation(context.Context, net.IP, *Client) error

	// ReserveStatic reserves an IPv4 address for a client, identified
	// by its hardware address, for as long as the database is open,
	// like a static address configured for the client: Reserve and
	// Lease fail with ErrAddressReserved for the IP address and any
	// other client, and succeed for the client without storing
	// anything. Static reservations are kept in memory only, so they
	// must be made again whenever the database is opened. Release
	// succeeds for a statically reserved IP address without doing
	// anything, DeleteReservation fails with ErrStaticReservation, and
	// ReservedAddresses returns static reservations without an
	// expiration time. ReserveStatic fails if the IP address or the
	// client already has a different static reservation
	ReserveStatic(context.Context, net.IP, Client) error

	// RemoveStaticConflicts removes the stored leases and reservations
	// of statically reserved IP addresses. Another client can't renew
	// its lease or reservation of such an address, and as a client has
	// a single lease or reservation, it would keep the client from
	// getting another address. The leases and reservations of other IP
	// addresses for a client with a static reservation are kept, as
	// the client uses its lease until it tries to renew it. DHCP
	// servers call RemoveStaticConflicts once they serve, so nothing
	// is removed if they fail to start
	RemoveStaticConflicts(context.Context) error
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
