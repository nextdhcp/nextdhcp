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

// clientID identifies a client in storages: by its hardware address.
type clientID string

// idOf returns the ID of cli.
func idOf(cli lease.Client) clientID {
	return clientID(cli.HwAddr.String())
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

// addressHandler reserves, leases and releases one IP address: Database
// decides once how to handle an address, and then calls its handler, a
// staticAddress for a statically reserved address and a storedAddress for
// any other.
type addressHandler interface {
	reserve(ctx context.Context, cli lease.Client) error
	leaseTo(ctx context.Context, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error)
	release(ctx context.Context) error
	deleteReservation(ctx context.Context, cli *lease.Client) error
}

// address returns the handler of ip.
func (db *Database) address(ip net.IP) addressHandler {
	if r, ok := db.static.get(ip); ok {
		return &staticAddress{db: db, r: r}
	}

	return &storedAddress{db: db, ip: ip}
}

// Reserve implements lease.Database
func (db *Database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	return db.address(ip).reserve(ctx, cli)
}

// Lease implements lease.Database
func (db *Database) Lease(ctx context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	return db.address(ip).leaseTo(ctx, cli, leaseTime, renew)
}

// Release implements lease.Database
func (db *Database) Release(ctx context.Context, ip net.IP) error {
	return db.address(ip).release(ctx)
}

// DeleteReservation implements lease.Database
func (db *Database) DeleteReservation(ctx context.Context, ip net.IP, cli *lease.Client) error {
	return db.address(ip).deleteReservation(ctx, cli)
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
	var errs []error
	for _, r := range db.static.list() {
		a := staticAddress{db: db, r: r}
		if err := a.removeConflict(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// takeOver removes the expired lease or reservation of ip for the client
// existing, so that the client id can get the address. Storages can't change
// the client of an entry, and hold one entry per client, so it keeps the
// expired entry, and fails with ErrDuplicateClientID, if the client id has an
// entry for another address.
func (db *Database) takeOver(ctx context.Context, ip net.IP, existing, id clientID) error {
	clientIP, _, _, err := db.store.FindByID(ctx, string(id))
	switch {
	case err == nil:
		return &ErrDuplicateClientID{ClientID: string(id), IP: clientIP}
	case !IsNotFound(err):
		return err
	}

	dhcpLog.With(ctx, db.l).Debugf("removing expired entry of IP %s for client %s", ip, existing)

	return db.deleteEntry(ctx, ip, existing)
}

// deleteEntry deletes the lease or reservation of ip for the client id from
// the storage, unless it's already gone.
func (db *Database) deleteEntry(ctx context.Context, ip net.IP, id clientID) error {
	if err := db.store.Delete(ctx, ip, string(id)); err != nil && !IsNotFound(err) {
		return err
	}

	return nil
}

// storedAddress handles an IP address whose leases and reservations are only
// stored.
type storedAddress struct {
	db *Database
	ip net.IP
}

func (a *storedAddress) reserve(ctx context.Context, cli lease.Client) error {
	db, ip := a.db, a.ip
	l := dhcpLog.With(ctx, db.l)

	id := idOf(cli)

	stored, leased, expiration, err := db.store.FindByIP(ctx, ip)
	existing := clientID(stored)
	if err != nil && !IsNotFound(err) {
		l.Debugf("failed to query IP storage for %s: %s", ip, err.Error())
		return err
	}

	if err == nil && existing != id {
		// IP address either leased or reserved for a different client
		if time.Now().Before(expiration) {
			l.Debugf("address %s already reserved for %s", ip, existing)
			return lease.ErrAddressReserved
		}

		// the entry of the other client expired, so reserve the IP address
		// for this client instead
		if err := db.takeOver(ctx, ip, existing, id); err != nil {
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
				if err := db.store.Update(ctx, ip, string(id), false, time.Now().Add(time.Minute)); err != nil {
					return err
				}
			} else {
				l.Warnf("Reserving already leased IP address %s for client %s", ip.String(), cli.String())
			}
		} else {
			l.Debugf("IP %s already reserved for %s", ip.String(), id)
		}

		return nil
	}

	return db.store.Create(ctx, ip, string(id), false, time.Now().Add(time.Minute))
}

func (a *storedAddress) leaseTo(ctx context.Context, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	db, ip := a.db, a.ip
	l := dhcpLog.With(ctx, db.l)

	id := idOf(cli)

	stored, leased, expiration, err := db.store.FindByIP(ctx, ip)
	existing := clientID(stored)
	if err != nil && !IsNotFound(err) {
		l.Errorf("failed to query lease storage for %s: %s", ip.String(), err.Error())
		return 0, err
	}

	if err == nil { // There's an existing lease/reservation for that IP
		if existing == id {
			// A reservation of the client becomes a lease with the full lease
			// time, and so does its lease if it's renewed or expired.
			// Otherwise the client keeps the lease it has, unless less than a
			// second remains: lease times are sent in whole seconds, so the
			// client would be told 0.
			remaining := time.Until(expiration)
			if !leased || renew || remaining < time.Second {
				newExpiration := time.Now().Add(leaseTime)
				l.Debugf("updating existing lease for IP %s (expiration=%s new-expiration=%s)", ip.String(), expiration, newExpiration)
				return leaseTime, db.store.Update(ctx, ip, string(id), true, newExpiration)
			}

			l.Debugf("using existing lease for IP %s", ip.String())

			return remaining, nil
		}

		// IP address already leased for a different client
		// we must not overwrite it if it's still valid
		if time.Now().Before(expiration) {
			l.Debugf("IP %s already leased for client %s", ip.String(), existing)
			return 0, lease.ErrAddressReserved
		}

		// IP lease already expired so we can take it over
		l.Infof("IP %s entry for client %s expired, overwritting (leased = %v)", ip, existing, leased)
		if err := db.takeOver(ctx, ip, existing, id); err != nil {
			return 0, err
		}

		// fallthrough
	}

	if err := db.store.Create(ctx, ip, string(id), true, time.Now().Add(leaseTime)); err != nil {
		l.Errorf("failed to lease IP %s for client %s: %s", ip.String(), id, err.Error())
		return 0, err
	}
	l.Debugf("leased IP %s for client %s", ip.String(), id)

	return leaseTime, nil
}

func (a *storedAddress) release(ctx context.Context) error {
	return a.db.store.Delete(ctx, a.ip, "")
}

func (a *storedAddress) deleteReservation(ctx context.Context, cli *lease.Client) error {
	var id clientID
	if cli != nil {
		id = idOf(*cli)
	}

	stored, leased, _, err := a.db.store.FindByIP(ctx, a.ip)
	if err != nil {
		return err
	}

	if id != "" && id != clientID(stored) {
		return ErrClientMismatch
	}

	if leased {
		return errors.New("reservation not found")
	}

	return a.db.store.Delete(ctx, a.ip, string(id))
}

// Interface guards
var (
	_ lease.Database       = (*Database)(nil)
	_ lease.StaticReserver = (*Database)(nil)
	_ addressHandler       = (*storedAddress)(nil)
	_ addressHandler       = (*staticAddress)(nil)
)
