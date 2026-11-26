package storage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	dhcpLog "github.com/nextdhcp/nextdhcp/core/log"
	"go.uber.org/zap"
)

// Database implements lease.Database
type Database struct {
	store LeaseStorage
	l     dhcpLog.Logger

	// staticLock protects static, which holds the static reservations by
	// the string form of their IP address. They are not persisted
	staticLock sync.RWMutex
	static     map[string]lease.ReservedAddress
}

// NewDatabase creates a new database that uses store for persistence. It
// discards the errors it logs until a logger is set with SetLogger.
func NewDatabase(store LeaseStorage) *Database {
	return &Database{
		store:  store,
		l:      zap.NewNop().Sugar(),
		static: make(map[string]lease.ReservedAddress),
	}
}

// SetLogger sets the logger that db logs errors to.
func (db *Database) SetLogger(l dhcpLog.Logger) {
	db.l = l
}

// Leases returns all IP address leases
func (db *Database) Leases(ctx context.Context) ([]lease.Lease, error) {
	ips, err := db.store.ListIPs(ctx)
	if err != nil {
		return nil, err
	}

	leases := make([]lease.Lease, 0, len(ips))
	for _, ip := range ips {
		cli, leased, expiration, err := db.store.FindByIP(ctx, ip)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil, err
			}

			db.l.Errorf("An error occured while loading IP lease for %s: %s", ip.String(), err.Error())
			continue
		}

		if !leased {
			continue
		}

		leases = append(leases, lease.Lease{
			Client: lease.Client{
				ID: cli,
			},
			Expires: expiration,
			Address: ip,
		})
	}

	return leases, nil
}

// ReservedAddresses returns all IP address leases
func (db *Database) ReservedAddresses(ctx context.Context) (lease.ReservedAddressList, error) {
	ips, err := db.store.ListIPs(ctx)
	if err != nil {
		return nil, err
	}

	leases := make(lease.ReservedAddressList, 0, len(ips))
	for _, ip := range ips {
		cli, leased, expiration, err := db.store.FindByIP(ctx, ip)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil, err
			}

			db.l.Errorf("An error occured while loading IP reservation for %s: %s", ip.String(), err.Error())
			continue
		}

		if leased {
			continue
		}

		leases = append(leases, lease.ReservedAddress{
			Client: lease.Client{
				ID: cli,
			},
			Expires: &expiration,
			IP:      ip,
		})
	}

	db.staticLock.RLock()
	defer db.staticLock.RUnlock()

	for _, r := range db.static {
		leases = append(leases, lease.ReservedAddress{
			Client: lease.Client{
				HwAddr:   append(net.HardwareAddr(nil), r.HwAddr...),
				Hostname: r.Hostname,
				ID:       r.ID,
			},
			IP: append(net.IP(nil), r.IP...),
		})
	}

	return leases, nil
}

// staticOwner returns the ID of the client that ip is statically reserved
// for, if it is.
func (db *Database) staticOwner(ip net.IP) (string, bool) {
	db.staticLock.RLock()
	defer db.staticLock.RUnlock()

	r, ok := db.static[ip.String()]

	return r.HwAddr.String(), ok
}

// ReserveStatic implements lease.Database
func (db *Database) ReserveStatic(ctx context.Context, ip net.IP, cli lease.Client) error {
	l := dhcpLog.With(ctx, db.l)

	// Storages compare IP addresses by their bytes, and store addresses in
	// the 4 byte form that DHCP messages use
	ip4 := ip.To4()
	if ip4 == nil {
		return lease.ErrInvalidAddress
	}
	ip4 = append(net.IP(nil), ip4...)

	if len(cli.HwAddr) == 0 {
		return errors.New("static reservations require the hardware address of the client")
	}
	clientID := cli.HwAddr.String()

	db.staticLock.Lock()
	defer db.staticLock.Unlock()

	if r, ok := db.static[ip4.String()]; ok {
		if owner := r.HwAddr.String(); owner != clientID {
			return fmt.Errorf("IP %s is already statically reserved for client %s", ip4, owner)
		}

		return nil
	}

	for _, r := range db.static {
		if r.HwAddr.String() == clientID {
			return fmt.Errorf("client %s already has a static reservation for IP %s", clientID, r.IP)
		}
	}

	// Remove the leases and reservations that contradict the static
	// reservation. Another client can't renew its lease or reservation of
	// the IP address anymore, and as storages hold a single lease or
	// reservation per client, it would keep that client from getting
	// another address. The client itself won't renew its leases or
	// reservations of other IP addresses, which would only keep them from
	// other clients until they expire.
	existingClient, _, _, err := db.store.FindByIP(ctx, ip4)
	switch {
	case err == nil:
		if existingClient != clientID {
			l.Warnf("removing lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", ip4, existingClient, clientID)
		}

		if err := db.store.Delete(ctx, ip4, existingClient); err != nil && !IsNotFound(err) {
			return err
		}

	case !IsNotFound(err):
		return err
	}

	existingIP, _, _, err := db.store.FindByID(ctx, clientID)
	switch {
	case err == nil:
		l.Infof("removing lease or reservation of IP %s for client %s: the client has the static IP %s", existingIP, clientID, ip4)

		if err := db.store.Delete(ctx, existingIP, clientID); err != nil && !IsNotFound(err) {
			return err
		}

	case !IsNotFound(err):
		return err
	}

	db.static[ip4.String()] = lease.ReservedAddress{
		Client: lease.Client{
			HwAddr:   append(net.HardwareAddr(nil), cli.HwAddr...),
			Hostname: cli.Hostname,
			ID:       cli.ID,
		},
		IP: ip4,
	}

	l.Debugf("statically reserved IP %s for client %s", ip4, clientID)

	return nil
}

// Reserve implements lease.Database
func (db *Database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	l := dhcpLog.With(ctx, db.l)

	clientID := cli.HwAddr.String()

	if owner, ok := db.staticOwner(ip); ok {
		if owner != clientID {
			l.Debugf("address %s statically reserved for %s", ip, owner)
			return lease.ErrAddressReserved
		}

		// static reservations don't expire
		return nil
	}

	existingClient, leased, expiration, err := db.store.FindByIP(ctx, ip)
	if err != nil && !IsNotFound(err) {
		l.Debugf("failed to query IP storage for %s: %s", ip, err.Error())
		return err
	}

	if err == nil { // !IsNotFound(err)
		if existingClient != clientID {
			// IP address either leased or ŕeserved for a different client
			if time.Now().Before(expiration) {
				l.Debugf("address %s already reserved for %s", ip, existingClient)
				return lease.ErrAddressReserved
			}
		}

		// The IP address is already leased  or reserved for this client. In case
		// the reservation expired we'll update it's validity
		//
		// TODO(ppacher): what to do if it's already leased?
		//
		if time.Now().After(expiration) {
			if !leased {
				l.Debugf("updating expired reservation for %s", ip)
				if err := db.store.Update(ctx, ip, clientID, false, time.Now().Add(time.Minute)); err != nil {
					return err
				}
			} else {
				l.Warnf("Reserving already leased IP address %s for client %s", ip.String(), cli.String())
			}
		} else {
			l.Debugf("IP %s already reserved for %s", ip.String(), clientID)
		}

		return nil
	}

	return db.store.Create(ctx, ip, clientID, false, time.Now().Add(time.Minute))
}

// Lease implementes lease.Database
func (db *Database) Lease(ctx context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	l := dhcpLog.With(ctx, db.l)

	clientID := cli.HwAddr.String()

	if owner, ok := db.staticOwner(ip); ok {
		if owner != clientID {
			l.Debugf("IP %s statically reserved for client %s", ip.String(), owner)
			return 0, lease.ErrAddressReserved
		}

		// leases of statically reserved addresses aren't stored
		return leaseTime, nil
	}

	existingClient, leased, expiration, err := db.store.FindByIP(ctx, ip)
	if err != nil && !IsNotFound(err) {
		l.Errorf("failed to query lease storage for %s: %s", ip.String(), err.Error())
		return 0, err
	}

	if err == nil { // There's an existing lease/reservation for that IP
		if existingClient == clientID {
			// address leased for this client
			// update lease time if requested or expired
			newExpiration := expiration
			activeLeaseTime := time.Until(expiration)
			update := false
			if renew || time.Now().After(expiration) {
				newExpiration = time.Now().Add(leaseTime)
				activeLeaseTime = leaseTime
				update = true
			}

			if !leased {
				update = true
			}

			if update {
				l.Debugf("updating existing lease for IP %s (expiration=%s new-expiration=%s)", ip.String(), expiration, newExpiration)
				return activeLeaseTime, db.store.Update(ctx, ip, existingClient, true, newExpiration)
			}
			l.Debugf("using existing lease for P %s", ip.String())

			return activeLeaseTime, nil
		}

		// IP address already leased for a different client
		// we must not overwrite it if it's still valid
		if time.Now().Before(expiration) {
			l.Debugf("IP %s already leased for client %s", ip.String(), existingClient)
			return 0, lease.ErrAddressReserved
		}

		// IP lease already expired so we can delete it
		l.Infof("IP %s entry for client %s expired, overwritting (leased = %v)", ip, cli, leased)
		if err := db.store.Delete(ctx, ip, existingClient); err != nil {
			return 0, err
		}

		// fallthrough
	}

	if err := db.store.Create(ctx, ip, clientID, true, time.Now().Add(leaseTime)); err != nil {
		l.Errorf("failed to lease IP %s for client %s: %s", ip.String(), clientID, err.Error())
		return 0, err
	}
	l.Debugf("leased IP %s for client %s", ip.String(), clientID)

	return leaseTime, nil
}

// DeleteReservation implements lease.Database
func (db *Database) DeleteReservation(ctx context.Context, ip net.IP, cli *lease.Client) error {
	clientID := ""
	if cli != nil {
		clientID = cli.HwAddr.String()
	}

	existingClient, leased, _, err := db.store.FindByIP(ctx, ip)
	if err != nil {
		return err
	}

	if clientID != "" && clientID != existingClient {
		return ErrClientMismatch
	}

	if leased {
		return errors.New("reservation not found")
	}

	return db.store.Delete(ctx, ip, clientID)
}

// Release implements lease.Database
func (db *Database) Release(ctx context.Context, ip net.IP) error {
	// leases of statically reserved addresses aren't stored, and releasing
	// them doesn't remove the static reservation
	if _, ok := db.staticOwner(ip); ok {
		return nil
	}

	return db.store.Delete(ctx, ip, "")
}
