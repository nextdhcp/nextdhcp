package bolt

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/etcd-io/bbolt"
	"github.com/nextdhcp/nextdhcp/core/lease"
)

var (
	leasesBucket      = []byte("leases")
	reservationBucket = []byte("reservations")
)

type (
	// Database is a lease.Database using bbolt to store
	// leases
	Database struct {
		*bbolt.DB
	}

	// binding is a IP address binding that is serialized
	// and stored in the database. It holds metadata about
	// the IP binding while the actual IP is used as the
	// key in bolt
	binding struct {
		Expires  int64  `json:"expires"`
		Created  int64  `json:"created"`
		MAC      string `json:"mac"`
		Hostname string `json:"hostname"`
		Addr     net.IP `json:"-"`
	}
)

// serialize serializes the binding into a byte slice
func (b *binding) serialize() ([]byte, error) {
	return json.Marshal(b)
}

// loadBinding unmarshals a binding from it's byte representation
func loadBinding(data []byte) (*binding, error) {
	var b binding

	err := json.Unmarshal(data, &b)
	return &b, err
}

// Setup prepares the boltdb by creating the leases bucket
func (db *Database) Setup() error {
	db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(leasesBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(reservationBucket)
		return err
	})
	return nil
}

// Leases implements lease.Database
func (db *Database) Leases(ctx context.Context) ([]lease.Lease, error) {
	bindings := db.loadBindings(ctx, leasesBucket)

	var leases []lease.Lease

	for b := range bindings {
		hw, _ := net.ParseMAC(b.MAC)
		leases = append(leases, lease.Lease{
			Address: b.Addr,
			Client: lease.Client{
				HwAddr:   hw,
				Hostname: b.Hostname,
			},
			Expires: time.Unix(b.Expires, 0),
		})
	}

	return leases, ctx.Err()
}

// ReservedAddresses implements lease.Database
func (db *Database) ReservedAddresses(ctx context.Context) (lease.ReservedAddressList, error) {
	bindings := db.loadBindings(ctx, reservationBucket)
	var addr []lease.ReservedAddress

	for b := range bindings {
		hw, _ := net.ParseMAC(b.MAC)
		expires := time.Unix(b.Expires, 0)

		addr = append(addr, lease.ReservedAddress{
			IP: b.Addr,
			Client: lease.Client{
				HwAddr:   hw,
				Hostname: b.Hostname,
			},
			Expires: &expires,
		})
	}

	return addr, ctx.Err()
}

// Reserve implements lease.Database
func (db *Database) Reserve(ctx context.Context, ip net.IP, cli lease.Client) error {
	return db.Update(func(tx *bbolt.Tx) error {
		key := []byte(ip)
		bucket := tx.Bucket(reservationBucket)
		existingBlob := bucket.Get(key)

		if existingBlob != nil {
			existing, err := loadBinding(existingBlob)
			if err != nil {
				return err
			}

			expired := time.Unix(existing.Expires, 0).Before(time.Now())
			if !expired {
				return lease.ErrAddressReserved
			} // else fallthrough and overwrite the existing reservation
		}

		b := binding{
			Created:  time.Now().Unix(),
			Expires:  time.Now().Add(time.Minute).Unix(),
			MAC:      cli.HwAddr.String(),
			Hostname: cli.Hostname,
		}
		value, err := b.serialize()
		if err != nil {
			return err
		}

		return bucket.Put(key, value)
	})
}

// Lease implements lease.Database
func (db *Database) Lease(ctx context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	var activeLeaseTime time.Duration

	err := db.Update(func(tx *bbolt.Tx) error {
		key := []byte(ip)
		bucket := tx.Bucket(leasesBucket)

		existingBlob := bucket.Get(key)
		if existingBlob != nil {
			existing, err := loadBinding(existingBlob)
			if err != nil {
				return err
			}
			// we need to treat expired leases as invalid and always need to renew them if possible
			expired := time.Unix(existing.Expires, 0).Before(time.Now())

			if existing.MAC == cli.HwAddr.String() {
				if renew || expired {
					existing.Expires = time.Now().Add(leaseTime).Unix()

					blob, err := existing.serialize()
					if err != nil {
						return err
					}

					if err := bucket.Put(key, blob); err != nil {
						return err
					}
					activeLeaseTime = leaseTime
				} else {
					activeLeaseTime = time.Unix(existing.Expires, 0).Sub(time.Now())
				}

				return nil
			}

			if !expired {
				return lease.ErrAddressReserved
			} // else fallthrough and overwrite the reservation for the new client
		}

		b := binding{
			Created:  time.Now().Unix(),
			Expires:  time.Now().Add(time.Minute).Unix(),
			MAC:      cli.HwAddr.String(),
			Hostname: cli.Hostname,
		}
		activeLeaseTime = time.Minute

		value, err := b.serialize()
		if err != nil {
			return err
		}

		return bucket.Put(key, value)
	})

	if err == nil {
		db.DeleteReservation(ctx, ip, &cli)
	}

	return activeLeaseTime, err
}

// Release implements lease.Database
func (db *Database) Release(ctx context.Context, ip net.IP) error {
	err := db.Update(func(tx *bbolt.Tx) error {
		key := []byte(ip)
		bucket := tx.Bucket(leasesBucket)

		if bucket.Get(key) == nil {
			return lease.ErrNoIPAvailable
		}

		return bucket.Delete(key)
	})

	resErr := db.DeleteReservation(ctx, ip, nil)
	if resErr != nil {
		if err == nil || err == lease.ErrNoIPAvailable {
			err = resErr
		}
	}

	return resErr
}

// DeleteReservation implements lease.Database
func (db *Database) DeleteReservation(_ context.Context, ip net.IP, cli *lease.Client) error {
	return db.Update(func(tx *bbolt.Tx) error {
		key := []byte(ip)
		bucket := tx.Bucket(reservationBucket)

		if bucket.Get(key) == nil {
			return lease.ErrNoIPAvailable
		}

		return bucket.Delete(key)
	})
}

func (db *Database) loadBindings(ctx context.Context, bucketName []byte) <-chan *binding {
	ch := make(chan *binding, 1)

	go func() {
		defer close(ch)
		db.View(func(tx *bbolt.Tx) error {
			c := tx.Bucket(bucketName).Cursor()

			for k, v := c.First(); k != nil; k, v = c.Next() {
				b, err := loadBinding(v)
				if err != nil {
					// ignore that for now
					continue
				}

				b.Addr = net.IP(k)
				select {
				case ch <- b:
				case <-ctx.Done():
					return nil
				}
			}

			return nil
		})
	}()

	return ch
}
