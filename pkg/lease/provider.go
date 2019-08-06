package lease

import (
	"net"
	"sync"
	"time"
)

type Storager interface {
	// All should return all leases stored by the storager
	All() []*Lease

	// Acquire is called when a lease is being claimed for a client
	Acquire(Lease) error

	// Release is called when a client lease expired or has been released
	// by the client
	Release(Lease) error
}

// Provider is capable of finding and creating IP address leases for clients. IP addresses are looked up
// from a list of IP ranges (start IP and end IP)
type Provider interface {
	// Network is the network that the provider manages
	Network() net.IPNet

	// All returns all leases
	All() []*Lease

	// CanLease checks whether the given IP address could be leased to the client. If the client
	// has already received a lease with the exact IP address true will be returned even if the
	// lease has expired
	CanLease(net.IPAddr, Client) bool

	// Find tries to find a lease for the client. If the client already has a lease it will
	// be returned even if it has been expired
	Find(Client) (*Lease, bool)

	// Lease IP to client for leaseTime
	Lease(IP net.IPAddr, client Client, leaseTime time.Duration) bool

	// Release releases a client lease
	Release(*Lease) bool

	// AddRange adds a new range to the lease pool
	AddRange(start, end net.IPAddr) error

	// RemoveRange removes the given range from the IP pool. Note that all active leases will
	// remain valid until they are released by the client or expire.
	RemoveRange(start, end net.IPAddr) error
}

// NewProvider returns a new IP address lease provider
func NewProvider(network net.IPNet, store Storager) Provider {
	/*
		return &provider{
			storager: store,
			network:  network,
		}
	*/
	return nil
}

type provider struct {
	network  net.IPNet     // the IP network served by the lease provider
	storager Storager      // used to persist leases
	mu       *sync.RWMutex // protects leases and ranges
	ranges   []*IPRange    // List of IP ranges leased by the provider
}

func (p *provider) Network() net.IPNet {
	return p.network
}

func (p *provider) All() []*Lease {
	return []*Lease{}
}
