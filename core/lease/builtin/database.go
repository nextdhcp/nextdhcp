package builtin

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/ppacher/dhcp-ng/core/lease"
	"github.com/ppacher/dhcp-ng/core/lease/iprange"
	"github.com/ppacher/webthings-mqtt-gateway/pkg/mutex"
)

// database implements the Database interface
type database struct {
	network                   *net.IPNet                       // IP network served by this database
	l                         *mutex.Mutex                     // context.Context aware mutex to protect all fields below
	ranges                    iprange.IPRanges                 // ranges usable for address leases
	reservedAddresses         map[uint32]lease.ReservedAddress // maps IP address to it's reserved address struct
	reservedAddressesByClient map[string]uint32                // maps a net.HardwareAddr.String() to the IP address reserved
	leasedAddresses           map[uint32]*lease.Lease          // maps IP address to lease
	leasedAddressesByClient   map[string]uint32                // maps net.HardwareAddr.String() to IP address
}

// New returns a new database instance
func New(nw *net.IPNet, ranges []*iprange.IPRange) lease.Database {
	// create a copy of the ranges slice
	rangesCpy := make([]*iprange.IPRange, len(ranges))
	for i, r := range ranges {
		rangesCpy[i] = r.Clone()
	}

	rangesCpy = iprange.Merge(rangesCpy)

	db := &database{
		l: mutex.New(),
		network: &net.IPNet{
			IP:   append(net.IP{}, nw.IP...),
			Mask: append(net.IPMask{}, nw.Mask...),
		},
		ranges:                    rangesCpy,
		reservedAddresses:         make(map[uint32]lease.ReservedAddress),
		reservedAddressesByClient: make(map[string]uint32),
		leasedAddresses:           make(map[uint32]*lease.Lease),
		leasedAddressesByClient:   make(map[string]uint32),
	}

	return db
}

func (db *database) Leases(ctx context.Context) ([]lease.Lease, error) {
	if !db.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer db.l.Unlock()

	var leases []lease.Lease
	for _, l := range db.leasedAddresses {
		leases = append(leases, *l.Clone())
	}

	return leases, nil
}

func (db *database) ReservedAddresses(ctx context.Context) ([]lease.ReservedAddress, error) {
	if !db.l.TryLock(ctx) {
		return nil, ctx.Err()
	}
	defer db.l.Unlock()

	var reservations []lease.ReservedAddress
	for _, l := range db.reservedAddresses {
		reservations = append(reservations, l)
	}

	return reservations, nil
}

func (db *database) FindAddress(ctx context.Context, cli *lease.Client) (net.IP, error) {
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
			key, _ := iprange.IP2Int(ip)

			if _, ok := db.reservedAddresses[key]; ok {
				continue
			}

			if _, ok := db.leasedAddresses[key]; ok {
				continue
			}

			return ip, nil
		}
	}

	return nil, lease.ErrNoIPAvailable
}

func (db *database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	if !db.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer db.l.Unlock()

	if !db.isLeasable(ip) {
		return errors.New("IP address not allowed")
	}

	key, ok := iprange.IP2Int(ip)
	if !ok {
		return lease.ErrInvalidAddress
	}

	if l, ok := db.leasedAddresses[key]; ok {
		if l.HwAddr.String() == cli.HwAddr.String() {
			return nil // already leased to the client
		}

		return lease.ErrAddressInUse
	}

	if r, ok := db.reservedAddresses[key]; ok {
		if r.HwAddr.String() == cli.HwAddr.String() {
			if r.Expired(time.Now()) {
				t := time.Now().Add(time.Minute)
				r.Expires = &t
			}

			return nil // already reserved for client
		}

		return lease.ErrAddressReserved
	}

	// TODO(ppacher): should we check for existing client reservations?
	// and maybe remove them?

	t := time.Now().Add(time.Minute)

	db.reservedAddresses[key] = lease.ReservedAddress{
		Client:  cli,
		IP:      ip,
		Expires: &t,
	}
	db.reservedAddressesByClient[cli.HwAddr.String()] = key

	return nil
}

func (db *database) Lease(ctx context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renewExisting bool) (time.Duration, error) {
	if !db.l.TryLock(ctx) {
		return 0, ctx.Err()
	}
	defer db.l.Unlock()

	if !db.isLeasable(ip) {
		return 0, errors.New("IP address not allowed")
	}

	key, ok := iprange.IP2Int(ip)
	if !ok {
		return 0, lease.ErrInvalidAddress
	}

	if l, ok := db.leasedAddresses[key]; ok {
		if l.HwAddr.String() == cli.HwAddr.String() {
			if renewExisting {
				l.Expires = time.Now().Add(leaseTime)
			}
			return l.Expires.Sub(time.Now()), nil
		}

		return 0, lease.ErrAddressInUse
	}

	if r, ok := db.reservedAddresses[key]; ok {
		if r.HwAddr.String() == cli.HwAddr.String() {
			if ip.String() != r.IP.String() {
				return 0, errors.New("reservation IP address missmatch")
			}

			delete(db.reservedAddresses, key)
			delete(db.reservedAddressesByClient, r.HwAddr.String())

			db.leasedAddresses[key] = &lease.Lease{
				Client:  cli,
				Address: ip,
				Expires: time.Now().Add(leaseTime),
			}
			db.leasedAddressesByClient[cli.HwAddr.String()] = key

			return leaseTime, nil
		}

		return 0, lease.ErrAddressReserved
	}

	return 0, lease.ErrNoIPAvailable
}

func (db *database) Release(ctx context.Context, ip net.IP) error {
	if !db.l.TryLock(ctx) {
		return ctx.Err()
	}
	defer db.l.Unlock()

	key, ok := iprange.IP2Int(ip)
	if !ok {
		return errors.New("invalid IPv4 address")
	}

	l, ok := db.leasedAddresses[key]
	if ok {
		delete(db.leasedAddresses, key)
		delete(db.leasedAddressesByClient, l.HwAddr.String())

		return nil
	}

	reservation, ok := db.reservedAddresses[key]
	if ok {
		delete(db.reservedAddresses, key)
		delete(db.reservedAddressesByClient, reservation.HwAddr.String())

		return nil
	}

	return lease.ErrNoIPAvailable
}

func (db *database) ReleaseClient(ctx context.Context, cli *lease.Client) error {
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

	return lease.ErrNoIPAvailable
}

func (db *database) AddRange(ranges ...*iprange.IPRange) error {
	db.l.Lock()
	defer db.l.Unlock()

	db.ranges = iprange.Merge(append(db.ranges, ranges...))

	return nil
}

func (db *database) DeleteRange(ranges ...*iprange.IPRange) error {
	db.l.Lock()
	defer db.l.Unlock()

	for _, r := range ranges {
		db.ranges = iprange.DeleteFrom(r, db.ranges)
	}

	return nil
}

func (db *database) DeleteReservation(ctx context.Context, ip net.IP, cli *lease.Client) error {
	db.l.Lock()
	defer db.l.Unlock()

	ipKey, ok := iprange.IP2Int(ip)
	if !ok {
		return lease.ErrInvalidAddress
	}

	reservation, ok := db.reservedAddresses[ipKey]
	if !ok {
		return lease.ErrNoIPAvailable
	}

	if cli != nil {
		if reservation.Client.HwAddr.String() != cli.HwAddr.String() {
			return errors.New("client MAC address mismatch")
		}
	}

	delete(db.reservedAddresses, ipKey)
	delete(db.reservedAddressesByClient, reservation.HwAddr.String())

	return nil
}

func (db *database) reservedAddrByCli(cli lease.Client) (lease.ReservedAddress, bool) {
	key := cli.HwAddr.String()
	ip, ok := db.reservedAddressesByClient[key]
	if !ok {
		return lease.ReservedAddress{}, false
	}

	r, ok := db.reservedAddresses[ip]

	return r, ok
}

func (db *database) leaseByCli(cli lease.Client) (*lease.Lease, bool) {
	key := cli.HwAddr.String()
	ip, ok := db.leasedAddressesByClient[key]
	if !ok {
		return nil, false
	}

	l, ok := db.leasedAddresses[ip]

	return l, ok
}

func (db *database) isLeasable(ip net.IP) bool {
	return iprange.IPRanges(db.ranges).Contains(ip)
}
