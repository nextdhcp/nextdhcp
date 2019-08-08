package lease

import (
	"log"
	"net"
	"sync"
	"time"
)

type Storager interface {
	// All should return all leases stored by the storager
	All() []*Lease

	// Acquire is called when a lease is being claimed for a client
	Acquire(*Lease) error

	// Release is called when a client lease expired or has been released
	// by the client
	Release(*Lease) error
}

// Provider is capable of finding and creating IP address leases for clients. IP addresses are looked up
// from a list of IP ranges (start IP and end IP)
type Provider interface {
	// Network is the network that the provider manages
	Network() *net.IPNet

	// All returns all leases
	All() []*Lease

	// CanLease checks whether the given IP address could be leased to the client. If the client
	// has already received a lease with the exact IP address true will be returned even if the
	// lease has expired
	CanLease(net.IP, Client) bool

	// Find searches for an existing client lease
	Find(Client) (*Lease, bool)

	// CreateLease allocates a new IP lease for the provided client. If the client already has
	// an active lease it will be renewed
	CreateLease(cli Client, leaseTime time.Duration) (*Lease, bool)

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

	// IPInPool reports whether the provider has the given IP in one of it's ranges
	IPInPool(ip net.IP) bool
}

// NewProvider returns a new IP address lease provider
func NewProvider(network *net.IPNet, store Storager) Provider {
	return &provider{
		storager:   store,
		network:    network,
		leasesByIP: make(map[uint32]*Lease),
		leasesByHW: make(map[string]*Lease),
	}
}

type provider struct {
	network    *net.IPNet   // the IP network served by the lease provider
	storager   Storager     // used to persist leases
	mu         sync.RWMutex // protects leases and ranges
	ranges     []*IPRange   // List of IP ranges leased by the provider
	leasesByIP map[uint32]*Lease
	leasesByHW map[string]*Lease
}

func (p *provider) Network() *net.IPNet {
	return p.network
}

func (p *provider) All() []*Lease {
	p.mu.RLock()

	leases := make([]*Lease, len(p.leasesByIP))
	idx := 0
	for _, l := range p.leasesByIP {
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
	// next we need to check if there's already a valid lease
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.canLease(ip, &cli)
}

func (p *provider) canLease(ip net.IP, cli *Client) bool {
	b, ok := IPToInt(ip)
	if !ok {
		return false
	}

	if l, ok := p.leasesByIP[b]; ok {
		if cli != nil && cli.HwAddr.String() == l.HwAddr.String() {
			return true
		}
		// IP address already assiged
		return false
	}

	return true
}

func (p *provider) Find(cli Client) (*Lease, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	lease, ok := p.leasesByHW[cli.HwAddr.String()]
	if ok {
		return lease, true
	}

	return nil, false
}

func (p *provider) CreateLease(cli Client, leaseTime time.Duration) (*Lease, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	lease, ok := p.leasesByHW[cli.HwAddr.String()]
	if ok {
		// we do have an active lease. Update the lease time if it expired
		if lease.Expired() {
			lease, ok := p.createLease(lease.Address, cli, leaseTime)
			if ok {
				return lease, ok
			}

			// we failed to re-lease the same address so fallthrough and create
			// a completely new one
		} else {
			// lease is still valid so allow the client to continue using it
			return lease, true
		}
	}

	// client does not have a lease. Try to find a new one
	for _, r := range p.ranges {
		for i := 0; i < r.Len(); i++ {
			ip := r.ByIdx(i)
			if p.canLease(ip, &cli) {
				lease, ok := p.createLease(ip, cli, leaseTime)
				if ok {
					return lease, ok
				}

				// if we failed to createLease the try the next one
			}
		}
	}

	return nil, false
}

func (p *provider) Lease(ip net.IP, cli Client, leaseTime time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.canLease(ip, &cli) {
		return false
	}

	_, leased := p.createLease(ip, cli, leaseTime)

	return leased
}

func (p *provider) createLease(ip net.IP, cli Client, leaseTime time.Duration) (*Lease, bool) {
	key, ok := IPToInt(ip)
	if !ok {
		return nil, false
	}

	lease := &Lease{
		Client:  cli,
		Address: ip,
		Expires: time.Now().Add(leaseTime).Unix(),
	}

	if p.storager != nil {
		if err := p.storager.Acquire(lease); err != nil {
			log.Printf("failed to acquire client lease: %s", err.Error())
			return nil, false
		}
	}

	p.leasesByIP[key] = lease
	p.leasesByHW[cli.HwAddr.String()] = lease

	return lease, true
}

func (p *provider) Release(l *Lease) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.storager != nil {
		if err := p.storager.Release(l); err != nil {
			log.Printf("failed to release lease: %s", err.Error())
			return false
		}
	}

	key, ok := IPToInt(l.Address)
	if !ok {
		return false
	}

	delete(p.leasesByIP, key)
	delete(p.leasesByHW, l.HwAddr.String())

	return true
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

func (p *provider) IPInPool(ip net.IP) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return IPRanges(p.ranges).Contains(ip)
}
