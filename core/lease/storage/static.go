package storage

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/nextdhcp/nextdhcp/core/lease"
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

	clientID := r.HwAddr.String()

	s.lock.Lock()
	defer s.lock.Unlock()

	if existing, ok := s.byIP[addr]; ok {
		if owner := existing.HwAddr.String(); owner != clientID {
			return fmt.Errorf("IP %s is already statically reserved for client %s", addr, owner)
		}

		return nil
	}

	for other, existing := range s.byIP {
		if existing.HwAddr.String() == clientID {
			return fmt.Errorf("client %s already has a static reservation for IP %s", clientID, other)
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
