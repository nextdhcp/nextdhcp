package storage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	dhcpLog "github.com/nextdhcp/nextdhcp/core/log"
)

// staticReservations holds the static reservations of a database, by their
// IPv4 address. They are kept in memory only.
type staticReservations struct {
	lock sync.RWMutex
	byIP map[netip.Addr]lease.ReservedAddress
}

// ipv4 returns ip as the IPv4 address that static reservations are kept by,
// and reports whether it is an IPv4 address.
func ipv4(ip net.IP) (netip.Addr, bool) {
	return netip.AddrFromSlice(ip.To4())
}

// get returns the static reservation of ip, if there is one. Its IP address
// is in the 4 byte form that storages store addresses in. Callers must not
// modify it.
func (s *staticReservations) get(ip net.IP) (lease.ReservedAddress, bool) {
	addr, ok := ipv4(ip)
	if !ok {
		return lease.ReservedAddress{}, false
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	r, ok := s.byIP[addr]

	return r, ok
}

// add adds a static reservation of r.IP for r.Client. It fails if r.IP isn't
// an IPv4 address, if r has no hardware address or has an expiration time,
// or if the IP address or the client already has a different static
// reservation.
func (s *staticReservations) add(r lease.ReservedAddress) error {
	addr, ok := ipv4(r.IP)
	if !ok {
		return fmt.Errorf("%w: %s is not an IPv4 address", lease.ErrInvalidAddress, r.IP)
	}

	if len(r.HwAddr) == 0 {
		return errors.New("static reservations require the hardware address of the client")
	}

	if r.Expires != nil {
		return errors.New("static reservations don't expire")
	}

	id := idOf(r.Client)

	s.lock.Lock()
	defer s.lock.Unlock()

	if existing, ok := s.byIP[addr]; ok {
		if owner := idOf(existing.Client); owner != id {
			return fmt.Errorf("IP %s is already statically reserved for client %s", addr, owner)
		}

		return nil
	}

	for other, existing := range s.byIP {
		if idOf(existing.Client) == id {
			return fmt.Errorf("client %s already has a static reservation for IP %s", id, other)
		}
	}

	if s.byIP == nil {
		s.byIP = make(map[netip.Addr]lease.ReservedAddress)
	}

	// storages compare IP addresses by their bytes, and store them in the
	// 4 byte form that DHCP messages use
	s.byIP[addr] = lease.ReservedAddress{Client: r.Client.Clone(), IP: net.IP(addr.AsSlice())}

	return nil
}

// list returns copies of the static reservations.
func (s *staticReservations) list() []lease.ReservedAddress {
	s.lock.RLock()
	defer s.lock.RUnlock()

	reservations := make([]lease.ReservedAddress, 0, len(s.byIP))
	for _, r := range s.byIP {
		reservations = append(reservations, r.Clone())
	}

	return reservations
}

// staticAddress handles a statically reserved IP address.
type staticAddress struct {
	db *Database
	r  lease.ReservedAddress // the reservation, with the 4 byte form of the address
}

// refuseOthers fails with lease.ErrAddressReserved if cli isn't the client
// the address is reserved for.
func (a *staticAddress) refuseOthers(ctx context.Context, cli lease.Client) error {
	owner := idOf(a.r.Client)
	if idOf(cli) == owner {
		return nil
	}

	l := dhcpLog.With(ctx, a.db.l)
	l.Debugf("IP %s statically reserved for client %s", a.r.IP, owner)

	// RemoveStaticConflicts removed the client's stored lease or reservation
	// of the address, unless it failed
	if err := a.removeConflict(ctx); err != nil {
		l.Errorf("failed to remove lease or reservation of statically reserved IP %s: %s", a.r.IP, err.Error())
	}

	return lease.ErrAddressReserved
}

func (a *staticAddress) reserve(ctx context.Context, cli lease.Client) error {
	// static reservations don't expire, so the client needs no stored one
	return a.refuseOthers(ctx, cli)
}

// leaseTo leases the address to the client it's reserved for, and stores
// the lease like any other, so that the address stays leased to the client
// until the lease expires if the static reservation is removed. A client
// that leases its static address gave up any other address, so its lease or
// reservation of another address is removed, as a client has a single one,
// and so are conflicts with the static reservation.
func (a *staticAddress) leaseTo(ctx context.Context, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	if err := a.refuseOthers(ctx, cli); err != nil {
		return 0, err
	}

	if err := a.removeConflict(ctx); err != nil {
		return 0, err
	}

	id := idOf(cli)

	otherIP, _, _, err := a.db.store.FindByID(ctx, string(id))
	switch {
	case err == nil && !otherIP.Equal(a.r.IP):
		dhcpLog.With(ctx, a.db.l).Infof("removing lease or reservation of IP %s for client %s: the client leases its static IP %s", otherIP, id, a.r.IP)

		if err := a.db.deleteEntry(ctx, otherIP, id); err != nil {
			return 0, err
		}

	case err != nil && !IsNotFound(err):
		return 0, err
	}

	stored := storedAddress{db: a.db, ip: a.r.IP}

	return stored.leaseTo(ctx, cli, leaseTime, renew)
}

// release keeps the static reservation and the stored lease of its client,
// and removes the leases and reservations of other clients that
// RemoveStaticConflicts didn't remove, maybe because it failed.
func (a *staticAddress) release(ctx context.Context) error {
	return a.removeConflict(ctx)
}

func (a *staticAddress) deleteReservation(context.Context, *lease.Client) error {
	return lease.ErrStaticReservation
}

// removeConflict removes the stored lease or reservation of the address,
// unless it's a lease of the client it's reserved for. Another client can't
// renew its lease or reservation of the address, and as a client has a
// single one, it would keep that client from getting another address.
func (a *staticAddress) removeConflict(ctx context.Context) error {
	stored, leased, _, err := a.db.store.FindByIP(ctx, a.r.IP)
	switch {
	case IsNotFound(err):
		return nil
	case err != nil:
		return err
	}

	existing, owner := clientID(stored), idOf(a.r.Client)
	if existing == owner && leased {
		return nil
	}

	if existing != owner {
		dhcpLog.With(ctx, a.db.l).Warnf("removing lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", a.r.IP, existing, owner)
	}

	return a.db.deleteEntry(ctx, a.r.IP, existing)
}
