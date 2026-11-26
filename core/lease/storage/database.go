package storage

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	dhcpLog "github.com/nextdhcp/nextdhcp/core/log"
	"go.uber.org/zap"
)

// Database implements lease.Database
type Database struct {
	store  LeaseStorage
	l      dhcpLog.Logger
	static staticReservations
}

// NewDatabase creates a new database that uses store for persistence. It
// discards the errors it logs until a logger is set with SetLogger.
func NewDatabase(store LeaseStorage) *Database {
	return &Database{
		store: store,
		l:     zap.NewNop().Sugar(),
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

	return append(leases, db.static.list()...), nil
}

// checkStatic reports whether ip is statically reserved, and returns
// lease.ErrAddressReserved if it is reserved for another client than the one
// with clientID.
func (db *Database) checkStatic(ctx context.Context, ip net.IP, clientID string) (bool, error) {
	r, ok := db.static.get(ip)
	if !ok {
		return false, nil
	}

	if owner := r.HwAddr.String(); owner != clientID {
		dhcpLog.With(ctx, db.l).Debugf("IP %s statically reserved for client %s", ip, owner)
		return true, lease.ErrAddressReserved
	}

	return true, nil
}

// takeOver removes the expired lease or reservation of ip for the client with
// existingClient, so that the client with clientID can get the address.
// Storages can't change the client of an entry, and hold one entry per
// client, so it keeps the expired entry, and fails with ErrDuplicateClientID,
// if the client with clientID has an entry for another address.
func (db *Database) takeOver(ctx context.Context, ip net.IP, existingClient, clientID string) error {
	clientIP, _, _, err := db.store.FindByID(ctx, clientID)
	switch {
	case err == nil:
		return &ErrDuplicateClientID{ClientID: clientID, IP: clientIP}
	case !IsNotFound(err):
		return err
	}

	dhcpLog.With(ctx, db.l).Debugf("removing expired entry of IP %s for client %s", ip, existingClient)

	return db.deleteEntry(ctx, ip, existingClient)
}

// deleteEntry deletes the lease or reservation of ip for the client with
// clientID from the storage, unless it's already gone.
func (db *Database) deleteEntry(ctx context.Context, ip net.IP, clientID string) error {
	if err := db.store.Delete(ctx, ip, clientID); err != nil && !IsNotFound(err) {
		return err
	}

	return nil
}

// ReserveStatic implements lease.StaticReserver
func (db *Database) ReserveStatic(ctx context.Context, r lease.ReservedAddress) error {
	if err := db.static.add(r); err != nil {
		return err
	}

	dhcpLog.With(ctx, db.l).Debugf("statically reserved IP %s for client %s", r.IP, r.HwAddr)

	return nil
}

// RemoveStaticConflicts implements lease.StaticReserver
func (db *Database) RemoveStaticConflicts(ctx context.Context) error {
	l := dhcpLog.With(ctx, db.l)

	var errs []error
	for _, r := range db.static.list() {
		existingClient, _, _, err := db.store.FindByIP(ctx, r.IP)
		switch {
		case IsNotFound(err):
			continue
		case err != nil:
			errs = append(errs, err)
			continue
		}

		if owner := r.HwAddr.String(); existingClient != owner {
			l.Warnf("removing lease or reservation of IP %s for client %s: the IP is statically reserved for client %s", r.IP, existingClient, owner)
		}

		if err := db.deleteEntry(ctx, r.IP, existingClient); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Reserve implements lease.Database
func (db *Database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	l := dhcpLog.With(ctx, db.l)

	clientID := cli.HwAddr.String()

	if static, err := db.checkStatic(ctx, ip, clientID); static {
		// static reservations don't expire
		return err
	}

	existingClient, leased, expiration, err := db.store.FindByIP(ctx, ip)
	if err != nil && !IsNotFound(err) {
		l.Debugf("failed to query IP storage for %s: %s", ip, err.Error())
		return err
	}

	if err == nil && existingClient != clientID {
		// IP address either leased or reserved for a different client
		if time.Now().Before(expiration) {
			l.Debugf("address %s already reserved for %s", ip, existingClient)
			return lease.ErrAddressReserved
		}

		// the entry of the other client expired, so reserve the IP address
		// for this client instead
		if err := db.takeOver(ctx, ip, existingClient, clientID); err != nil {
			return err
		}
	} else if err == nil {
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

	if static, err := db.checkStatic(ctx, ip, clientID); static {
		if err != nil {
			return 0, err
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
			// A reservation of the client becomes a lease with the full lease
			// time, and so does its lease if it's renewed or expired.
			// Otherwise the client keeps the lease it has, unless less than a
			// second remains: lease times are sent in whole seconds, so the
			// client would be told 0.
			remaining := time.Until(expiration)
			if !leased || renew || remaining < time.Second {
				newExpiration := time.Now().Add(leaseTime)
				l.Debugf("updating existing lease for IP %s (expiration=%s new-expiration=%s)", ip.String(), expiration, newExpiration)
				return leaseTime, db.store.Update(ctx, ip, existingClient, true, newExpiration)
			}

			l.Debugf("using existing lease for IP %s", ip.String())

			return remaining, nil
		}

		// IP address already leased for a different client
		// we must not overwrite it if it's still valid
		if time.Now().Before(expiration) {
			l.Debugf("IP %s already leased for client %s", ip.String(), existingClient)
			return 0, lease.ErrAddressReserved
		}

		// IP lease already expired so we can take it over
		l.Infof("IP %s entry for client %s expired, overwritting (leased = %v)", ip, cli, leased)
		if err := db.takeOver(ctx, ip, existingClient, clientID); err != nil {
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
	if _, ok := db.static.get(ip); ok {
		return lease.ErrStaticReservation
	}

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
	// Leases of statically reserved addresses aren't stored, and releasing
	// them doesn't remove the static reservation. A stored lease of such an
	// address is one that RemoveStaticConflicts didn't remove, maybe because
	// it failed, so remove it.
	if r, ok := db.static.get(ip); ok {
		return db.deleteEntry(ctx, r.IP, "")
	}

	return db.store.Delete(ctx, ip, "")
}

// Interface guards
var (
	_ lease.Database       = (*Database)(nil)
	_ lease.StaticReserver = (*Database)(nil)
)
