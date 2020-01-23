package bolt

import (
	"encoding/json"
	"net"
	"time"

	"go.etcd.io/bbolt"
)

var (
	v0LeasesBucket      = []byte("leases")
	v0ReservationBucket = []byte("reservations")
)

type (
	v0Binding struct {
		Expires  int64  `json:"expires"`
		Created  int64  `json:"created"`
		MAC      string `json:"mac"`
		Hostname string `json:"hostname"`
		Addr     net.IP `json:"-"`
	}
)

// v0ToV1 migrates the bolt db from it's pervious format to the new one
func v0ToV1(db *bbolt.Tx) error {
	leases := db.Bucket(v0LeasesBucket)
	if leases != nil {
		if err := v0_1MigrateBucket(db, leases, true); err != nil {
			return err
		}
		if err := db.DeleteBucket(v0LeasesBucket); err != nil {
			return err
		}
	}

	reservations := db.Bucket(v0ReservationBucket)
	if reservations != nil {
		if err := v0_1MigrateBucket(db, reservations, false); err != nil {
			return err
		}
		if err := db.DeleteBucket(v0ReservationBucket); err != nil {
			return err
		}
	}

	return db.Bucket(schemaVersionBucket).Put(schemaVersionKey, []byte("1"))
}

func v0_1MigrateBucket(db *bbolt.Tx, bucket *bbolt.Bucket, leased bool) error {
	cursor := bucket.Cursor()
	key, value := cursor.First()

	// the old bolt schema allowed stale entries to be kept in the database
	// (which also caused some problems). During migration, we'll keep
	// the freshes leases (even if they are expired)
	clientLeases := make(map[string]*entry)
	clientIPs := make(map[string]net.IP)

	for key != nil {
		e, err := v0_1MigrateBinding(key, value, leased)
		if err != nil {
			return err
		}

		if !leased {
			// we skip expired reservations
			// but not expired leases
			if time.Unix(e.Expires, 0).Before(time.Now()) {
				key, value = cursor.Next()
				continue
			}
		}

		if existing, ok := clientLeases[e.ClientID]; ok {
			if existing.Expires > e.Expires {
				key, value = cursor.Next()
				continue
			}
		}
		clientLeases[e.ClientID] = e
		clientIPs[e.ClientID] = net.IP(key)

		key, value = cursor.Next()
	}

	for key, e := range clientLeases {
		ip := clientIPs[key]
		if err := v0_1InsertEntry(db, ip, e); err != nil {
			return err
		}
	}

	return nil
}

func v0_1MigrateBinding(key, value []byte, leased bool) (*entry, error) {
	var old v0Binding

	if err := json.Unmarshal(value, &old); err != nil {
		return nil, err
	}

	return &entry{
		Expires:  old.Expires,
		Leased:   leased,
		ClientID: old.MAC,
	}, nil
}

func v0_1InsertEntry(tx *bbolt.Tx, ip net.IP, e *entry) error {
	blob, err := json.Marshal(e)
	if err != nil {
		return err
	}

	ipLeasesBucket, idToIPBucket, err := openOrCreateBuckets(tx)
	if err != nil {
		return err
	}

	if err := idToIPBucket.Put([]byte(e.ClientID), []byte(ip)); err != nil {
		return err
	}

	return ipLeasesBucket.Put([]byte(ip), blob)
}
