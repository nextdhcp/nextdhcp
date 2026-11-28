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

// staticAddress handles a statically reserved IP address. Another client may
// still use a lease of the address that hasn't expired, from before it was
// reserved, so the address is only leased to its client once that lease
// ends: when the other client asks for an address, which it is refused, or
// when the lease expires.
type staticAddress struct {
	db *Database
	r  lease.ReservedAddress // the reservation, with the 4 byte form of the address
}

// refuseOthers fails with lease.ErrAddressReserved if cli isn't the client
// the address is reserved for. Such a client doesn't use a lease of the
// address anymore once it asks for an address and is refused, so its stored
// lease or reservation of the address is removed, as a client has a single
// one, and it would keep the client from getting another address.
func (a *staticAddress) refuseOthers(ctx context.Context, cli lease.Client) error {
	owner, id := idOf(a.r.Client), idOf(cli)
	if id == owner {
		return nil
	}

	l := dhcpLog.With(ctx, a.db.l)
	l.Debug().Msgf("IP %s statically reserved for client %s", a.r.IP, owner)

	// Database refuses clients without hardware address, whose empty client
	// ID would delete the entry of any client
	switch err := a.db.store.Delete(ctx, a.r.IP, id); {
	case err == nil:
		l.Info().Msgf("removed lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", a.r.IP, id, owner)
	case !IsNotFound(err) && !errors.Is(err, ErrClientMismatch):
		l.Error().Msgf("failed to remove lease or reservation of statically reserved IP %s: %s", a.r.IP, err.Error())
	}

	return lease.ErrAddressReserved
}

// prepare prepares the address for the client it's reserved for. It fails
// with lease.ErrAddressReserved while another client has a lease of it that
// hasn't expired, and removes other leases and reservations of other clients
// of the address, which they don't use.
func (a *staticAddress) prepare(ctx context.Context) error {
	existing, leased, expiration, err := a.db.store.FindByIP(ctx, a.r.IP)
	switch {
	case IsNotFound(err):
		return nil
	case err != nil:
		return err
	}

	owner := idOf(a.r.Client)
	if existing == owner {
		return nil
	}

	l := dhcpLog.With(ctx, a.db.l)
	if leased && time.Now().Before(expiration) {
		l.Debug().Msgf("static IP %s of client %s is leased to client %s until %s", a.r.IP, owner, existing, expiration)
		return lease.ErrAddressReserved
	}

	l.Warn().Msgf("removing lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", a.r.IP, existing, owner)

	return a.db.deleteEntry(ctx, a.r.IP, existing)
}

func (a *staticAddress) reserve(ctx context.Context, cli lease.Client) error {
	if err := a.refuseOthers(ctx, cli); err != nil {
		return err
	}

	// static reservations don't expire, so the client needs no stored one
	return a.prepare(ctx)
}

// leaseTo leases the address to the client it's reserved for, and stores
// the lease like any other, so that the address stays leased to the client
// until the lease expires if the static reservation is removed. A client
// that leases its static address gave up any other address, so its lease or
// reservation of another address is removed, as a client has a single one.
func (a *staticAddress) leaseTo(ctx context.Context, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	if err := a.refuseOthers(ctx, cli); err != nil {
		return 0, err
	}

	if err := a.prepare(ctx); err != nil {
		return 0, err
	}

	id := idOf(cli)

	otherIP, _, _, err := a.db.store.FindByID(ctx, id)
	switch {
	case err == nil && !otherIP.Equal(a.r.IP):
		l := dhcpLog.With(ctx, a.db.l)
		l.Info().Msgf("removing lease or reservation of IP %s for client %s: the client leases its static IP %s", otherIP, id, a.r.IP)

		if err := a.db.deleteEntry(ctx, otherIP, id); err != nil {
			return 0, err
		}

	case err != nil && !IsNotFound(err):
		return 0, err
	}

	stored := storedAddress{db: a.db, ip: a.r.IP}

	return stored.leaseTo(ctx, cli, leaseTime, renew)
}

// release keeps the static reservation and the stored lease or reservation
// of its client, and removes the lease or reservation of another client,
// which gives the address up. Given a client, it fails with
// ErrClientMismatch if the stored lease or reservation belongs to another
// client.
func (a *staticAddress) release(ctx context.Context, cli *lease.Client) error {
	existing, _, _, err := a.db.store.FindByIP(ctx, a.r.IP)
	switch {
	case IsNotFound(err):
		return nil
	case err != nil:
		return err
	}

	if cli != nil && idOf(*cli) != existing {
		return ErrClientMismatch
	}

	owner := idOf(a.r.Client)
	if existing == owner {
		return nil
	}

	l := dhcpLog.With(ctx, a.db.l)
	l.Info().Msgf("removing lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", a.r.IP, existing, owner)

	return a.db.deleteEntry(ctx, a.r.IP, existing)
}

func (a *staticAddress) deleteReservation(context.Context, *lease.Client) error {
	return lease.ErrStaticReservation
}

// decline fails with lease.ErrStaticReservation for the client the address
// is reserved for. Another client gives the address up, which the static
// reservation already keeps from other clients, so only its lease or
// reservation is removed, which fails if it's another client's.
func (a *staticAddress) decline(ctx context.Context, cli lease.Client, _ time.Time) error {
	if idOf(cli) == idOf(a.r.Client) {
		return lease.ErrStaticReservation
	}

	return a.db.store.Delete(ctx, a.r.IP, idOf(cli))
}
