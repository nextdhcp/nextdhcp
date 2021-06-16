package storage

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/apex/log"
	"github.com/nextdhcp/nextdhcp/core/lease"
	dhcpLog "github.com/nextdhcp/nextdhcp/core/log"
)

// Database implements lease.Database
type Database struct {
	store LeaseStorage
	l     dhcpLog.Logger
}

// NewDatabase creates a new database that uses store for persistence
func NewDatabase(store LeaseStorage) *Database {
	return &Database{
		store: store,
		l:     log.Log,
	}
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

	return leases, nil
}

// Reserve implements lease.Database
func (db *Database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	l := dhcpLog.With(ctx, db.l)

	clientID := cli.HwAddr.String()

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
			activeLeaseTime := expiration.Sub(time.Now())
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

	if !leased {
		return errors.New("reservation not found")
	}

	return db.store.Delete(ctx, ip, clientID)
}

// Release implements lease.Database
func (db *Database) Release(ctx context.Context, ip net.IP) error {
	return db.store.Delete(ctx, ip, "")
}
