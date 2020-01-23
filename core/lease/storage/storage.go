package storage

import (
	"context"
	"net"
	"time"
)

// LeaseStorage provides persistence for IP addresses
// leased or reserved to clients. Implementations don't
// need to care about lease management mechanics and just
// need to provide a way to keep leases persistent across
// service and host restarts. This also means that storage
// implementations don't, and should not, interpret the leased
// and expiration members. They are only required to ensure
// IP addresses and clientIDs are not used more than once
// (unique index)
type LeaseStorage interface {
	// Create stores a unique IP address lease.
	// Implementation MUST check that  IP and client
	// are not already used inside the database
	Create(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error

	// Delete deletes an IP lease from the database.
	// If clientID is not empty, implementations should
	// check if the lease to be removed is for the same clientID
	Delete(ctx context.Context, ip net.IP, clientID string) error

	// Update updates `leased` and `expiration` of an
	// existing IP lease. The operation should only be performed
	// if clientID matches the stored one.
	Update(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error

	// FindByIP searches for an IP lease for the given IP
	FindByIP(ctx context.Context, ip net.IP) (clientID string, leased bool, expiration time.Time, err error)

	// FindByID searches for an IP lease for the given client ID
	FindByID(ctx context.Context, clientID string) (ip net.IP, leased bool, expiration time.Time, err error)

	// ListIPs returns a list of IPs that are available in the storage
	ListIPs(ctx context.Context) ([]net.IP, error)

	// ListIDs returns a list of client IDs that are available in the storage
	ListIDs(ctx context.Context) ([]string, error)
}
