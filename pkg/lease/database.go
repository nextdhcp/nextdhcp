package lease

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/ppacher/webthings-mqtt-gateway/pkg/mutex"
)

// ReservedAddress represents an IPv4 address that has been reserved for a specific client.
// A reserved address may have an expiration time
type ReservedAddress struct {
	// Client is the client for which IP has been reserved
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

	// ReleaseClient releases all IP address leases or reservations for the given client
	ReleaseClient(context.Context, *Client) error

	// AddRange adds new ranges to the list of IP addresses that can be leased
	AddRange(ranges ...*IPRange)

	// DeleteRange deletes ranges from the list of leasable IP addresses. Already leased addreses
	// will still be valid until they expire
	DeleteRange(ranges ...*IPRange)
}

// database implements the Database interface
type database struct {
	network                   *net.IPNet                 // IP network served by this database
	l                         *mutex.Mutex               // context.Context aware mutex to protect all fields below
	ranges                    IPRanges                   // ranges usable for address leases
	reservedAddresses         map[uint32]ReservedAddress // maps IP address to it's reserved address struct
	reservedAddressesByClient map[string]uint32          // maps a net.HardwareAddr.String() to the IP address reserved
	leasedAddresses           map[uint32]*Lease          // maps IP address to lease
	leasedAddressesByClient   map[string]uint32          // maps net.HardwareAddr.String() to IP address
}

// Option is a database option
type Option func(d *database)

// WithRange is a database option to configure a list of IP ranges that
// can be leased
func WithRange(ranges ...*IPRange) Option {
	return func(db *database) {
		db.ranges = mergeConsecutiveRanges(append(db.ranges, ranges...))
	}
}

// NewDatabase returns a new database instance
func NewDatabase(nw *net.IPNet, ranges []*IPRange, options ...Option) Database {
	// create a copy of the ranges slice
	rangesCpy := make([]*IPRange, len(ranges))
	for i, r := range ranges {
		rangesCpy[i] = r.Clone()
	}

	db := &database{
		l: mutex.New(),
		network: &net.IPNet{
			IP:   append(net.IP{}, nw.IP...),
			Mask: append(net.IPMask{}, nw.Mask...),
		},
		ranges:                    rangesCpy,
		reservedAddresses:         make(map[uint32]ReservedAddress),
		reservedAddressesByClient: make(map[string]uint32),
		leasedAddresses:           make(map[uint32]*Lease),
		leasedAddressesByClient:   make(map[string]uint32),
	}

	for _, opt := range options {
		opt(db)
	}

	return db
}

func (db *database) Leases(ctx context.Context) ([]Lease, error) {
	if !db.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer db.l.Unlock()

	var leases []Lease
	for _, l := range db.leasedAddresses {
		leases = append(leases, *l.Clone())
	}

	return leases, nil
}

func (db *database) ReservedAddresses(ctx context.Context) ([]ReservedAddress, error) {
	if !db.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer db.l.Unlock()

	var reservations []ReservedAddress
	for _, l := range db.reservedAddresses {
		reservations = append(reservations, l)
	}

	return reservations, nil
}

func (db *database) FindAddress(ctx context.Context, cli *Client) (net.IP, error) {
	if !db.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer db.l.Unlock()

	l, ok := db.leaseByCli(*cli)
	if ok {
		return l.Address, nil
	}

	r, ok := db.reservedAddrByCli(*cli)
	if ok {
		return r.IP, nil
	}

	for _, ipRange := range db.ranges {
		for i := 0; i < ipRange.Len(); i++ {
			ip := ipRange.ByIdx(i)
			key, _ := IPToInt(ip)

			if _, ok := db.reservedAddresses[key]; ok {
				continue
			}

			if _, ok := db.leasedAddresses[key]; ok {
				continue
			}

			return ip, nil
		}
	}

	return nil, errors.New("no leasable addresses")
}

func (db *database) Reserve(ctx context.Context, ip net.IP, cli Client) error {
	if !db.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer db.l.Unlock()

	if !db.isLeasable(ip) {
		return errors.New("IP address not allowed")
	}

	key, ok := IPToInt(ip)
	if !ok {
		return errors.New("invalid ip address")
	}

	if l, ok := db.leasedAddresses[key]; ok {
		if l.HwAddr.String() == cli.HwAddr.String() {
			return nil // already leased to the client
		}

		return errors.New("address already leased")
	}

	if r, ok := db.reservedAddresses[key]; ok {
		if r.HwAddr.String() == cli.HwAddr.String() {
			if r.Expired(time.Now()) {
				t := time.Now().Add(time.Minute)
				r.Expires = &t
			}

			return nil // already reserved for client
		}

		return errors.New("address already reserved")
	}

	// TODO(ppacher): should we check for existing client reservations?
	// and maybe remove them?

	t := time.Now().Add(time.Minute)

	db.reservedAddresses[key] = ReservedAddress{
		Client:  cli,
		IP:      ip,
		Expires: &t,
	}
	db.reservedAddressesByClient[cli.HwAddr.String()] = key

	return nil
}

func (db *database) Lease(ctx context.Context, ip net.IP, cli Client, leaseTime time.Duration, renewExisting bool) (time.Duration, error) {
	if !db.l.TryLock(ctx) {
		return 0, ctx.Err()
	}
	defer db.l.Unlock()

	if !db.isLeasable(ip) {
		return 0, errors.New("IP address not allowed")
	}

	key, ok := IPToInt(ip)
	if !ok {
		return 0, errors.New("invalid IP address")
	}

	if l, ok := db.leasedAddresses[key]; ok {
		if l.HwAddr.String() == cli.HwAddr.String() {
			if renewExisting {
				l.Expires = time.Now().Add(leaseTime)
			}
			return l.Expires.Sub(time.Now()), nil
		}

		return 0, errors.New("IP address in use")
	}

	if r, ok := db.reservedAddresses[key]; ok {
		if r.HwAddr.String() == cli.HwAddr.String() {
			if ip.String() != r.IP.String() {
				return 0, errors.New("reservation IP address missmatch")
			}

			delete(db.reservedAddresses, key)
			delete(db.reservedAddressesByClient, r.HwAddr.String())

			db.leasedAddresses[key] = &Lease{
				Client:  cli,
				Address: ip,
				Expires: time.Now().Add(leaseTime),
			}
			db.leasedAddressesByClient[cli.HwAddr.String()] = key

			return leaseTime, nil
		}

		return 0, errors.New("IP address reserved for a different client")
	}

	return 0, errors.New("no reservation for IP address available")
}

func (db *database) Release(ctx context.Context, ip net.IP) error {
	if !db.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer db.l.Unlock()

	key, ok := IPToInt(ip)
	if !ok {
		return errors.New("invalid IPv4 address")
	}

	lease, ok := db.leasedAddresses[key]
	if ok {
		delete(db.leasedAddresses, key)
		delete(db.leasedAddressesByClient, lease.HwAddr.String())

		return nil
	}

	reservation, ok := db.reservedAddresses[key]
	if ok {
		delete(db.reservedAddresses, key)
		delete(db.reservedAddressesByClient, reservation.HwAddr.String())

		return nil
	}

	return errors.New("unknown lease or reservation")
}

func (db *database) ReleaseClient(ctx context.Context, cli *Client) error {
	if !db.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer db.l.Unlock()

	key := cli.HwAddr.String()

	idx, ok := db.leasedAddressesByClient[key]
	if ok {
		delete(db.leasedAddresses, idx)
		delete(db.leasedAddressesByClient, key)

		return nil
	}

	idx, ok = db.reservedAddressesByClient[key]
	if ok {
		delete(db.reservedAddresses, idx)
		delete(db.reservedAddressesByClient, key)

		return nil
	}

	return errors.New("unknown lease or reservation")
}

func (db *database) AddRange(ranges ...*IPRange) {
	db.l.Lock()
	defer db.l.Unlock()

	db.ranges = mergeConsecutiveRanges(append(db.ranges, ranges...))
}

func (db *database) DeleteRange(ranges ...*IPRange) {
	db.l.Lock()
	defer db.l.Unlock()

	for _, r := range ranges {
		db.ranges = deleteRange(r, db.ranges)
	}
}

func (db *database) reservedAddrByCli(cli Client) (ReservedAddress, bool) {
	key := cli.HwAddr.String()
	ip, ok := db.reservedAddressesByClient[key]
	if !ok {
		return ReservedAddress{}, false
	}

	r, ok := db.reservedAddresses[ip]

	return r, ok
}

func (db *database) leaseByCli(cli Client) (*Lease, bool) {
	key := cli.HwAddr.String()
	ip, ok := db.leasedAddressesByClient[key]
	if !ok {
		return nil, false
	}

	l, ok := db.leasedAddresses[ip]

	return l, ok
}

func (db *database) isLeasable(ip net.IP) bool {
	return IPRanges(db.ranges).Contains(ip)
}
