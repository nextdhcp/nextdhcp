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
	CanLease(net.IP, Client) bool

	// Find tries to find a lease for the client. If the client already has a lease it will
	// be returned even if it has been expired
	Find(Client) (*Lease, bool)

	// Lease IP to client for leaseTime
	Lease(IP net.IP, client Client, leaseTime time.Duration) bool

	// Release releases a client lease
	Release(*Lease) bool

	// AddRange adds a new range to the lease pool
	AddRange(start, end net.IP) error

	// RemoveRange removes the given range from the IP pool. Note that all active leases will
	// remain valid until they are released by the client or expire.
	RemoveRange(start, end net.IP) error

	// PoolSize returns the total number of IP addresses available
	PoolSize() int
}

// NewProvider returns a new IP address lease provider
func NewProvider(network net.IPNet, store Storager) Provider {
	return &provider{
		storager: store,
		network:  network,
	}
}

type provider struct {
	network  net.IPNet     // the IP network served by the lease provider
	storager Storager      // used to persist leases
	mu       *sync.RWMutex // protects leases and ranges
	ranges   []*IPRange    // List of IP ranges leased by the provider
	leases   map[uint32]*Lease
}

func (p *provider) Network() net.IPNet {
	return p.network
}

func (p *provider) All() []*Lease {
	p.mu.RLock()

	leases := make([]*Lease, len(p.leases))
	idx := 0
	for _, l := range p.leases {
		leases[idx] = l
		idx = idx + 1
	}

	p.mu.RUnlock()

	copy := make([]*Lease, len(leases))
	for i, l := range leases {
		copy[i] = l.Clone()
	}

	return copy
}

func (p *provider) CanLease(ip net.IP, cli Client) bool {
	// first we need to check if the requested IP is at least
	// in the network we are managing
	if !p.network.Contains(ip) {
		return false
	}

	ipv4, ok := IPToInt(ip)
	if !ok {
		return false
	}

	// next we need to check if there's already a valid lease
	p.mu.RLock()
	defer p.mu.RUnlock()

	for key, lease := range p.leases {
		if key == ipv4 {
			// if it's the same client we can reuse the lease
			if lease.Client.HwAddr.String() == cli.HwAddr.String() {
				return true
			}

			return false
		}
	}

	// seems like no one is using this lease
	// TODO(ppacher): should we check the lease against the list of valid ranges?
	return true
}

func (p *provider) Find(cli Client) (*Lease, bool) {
	panic("not yet implemented")
}

func (p *provider) Lease(IP net.IP, cli Client, leaseTime time.Duration) bool {
	panic("not yet implemented")
}

func (p *provider) Release(l *Lease) bool {
	panic("not yet implemented")
}

func (p *provider) AddRange(start, end net.IP) error {
	r := &IPRange{start, end}
	if err := r.Validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	ranges := append(p.ranges, r)
	p.ranges = mergeConsecutiveRanges(ranges)

	return nil
}

func (p *provider) RemoveRange(start, end net.IP) error {
	r := &IPRange{start, end}
	if err := r.Validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.ranges = deleteRange(r, p.ranges)
	return nil
}

func (p *provider) PoolSize() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sum := 0
	for _, r := range p.ranges {
		sum += r.Len()
	}

	return sum
}
