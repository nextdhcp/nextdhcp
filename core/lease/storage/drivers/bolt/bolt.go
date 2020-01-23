package bolt

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"go.etcd.io/bbolt"
)

var (
	ipLeaseBucketKey = []byte("ip-leases")
	idToIPBucketKey  = []byte("id-to-ip-bucket")
)

// SchemaVersion is the current version of the bolt db
const SchemaVersion = "1"

type (
	// Storage is a storage.LeaseStorage implementation that persists
	// IP leases in a bbolt database
	Storage struct {
		db   *bbolt.DB
		path string
	}

	entry struct {
		Expires  int64  `json:"expires"`
		ClientID string `json:"clientID"`
		Leased   bool   `json:"leased"`
	}
)

// Create implements lease.Storage
func (s *Storage) Create(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		ipLeaseBucket, idToIPBucket, err := openOrCreateBuckets(tx)
		if err != nil {
			return err
		}
		if err := assertUniqueClient(idToIPBucket, clientID, ip); err != nil {
			return err
		}
		if err := assertUniqueIP(ipLeaseBucket, ip, clientID); err != nil {
			return err
		}
		if ipLeaseBucket.Get([]byte(ip)) != nil {
			// the same IP/clientID pair is already stored
			return &storage.ErrDuplicateIP{IP: ip, ClientID: clientID}
		}

		e := entry{
			Expires:  expiration.Unix(),
			ClientID: clientID,
			Leased:   leased,
		}
		blob, err := json.Marshal(e)
		if err != nil {
			return err
		}

		if err := idToIPBucket.Put([]byte(clientID), []byte(ip)); err != nil {
			return err
		}
		return ipLeaseBucket.Put([]byte(ip), blob)
	})
}

// Delete implements storage.LeaseStorage
func (s *Storage) Delete(ctx context.Context, ip net.IP, clientID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		ipLeaseBucket, idToIPBucket, err := openOrCreateBuckets(tx)
		if err != nil {
			return err
		}

		blob := ipLeaseBucket.Get([]byte(ip))
		if blob == nil {
			return &storage.ErrIPNotFound{IP: ip}
		}

		var e entry
		if err := json.Unmarshal(blob, &e); err != nil {
			return err
		}

		if clientID != "" && e.ClientID != clientID {
			return storage.ErrClientMismatch
		}

		if err := ipLeaseBucket.Delete([]byte(ip)); err != nil {
			return err
		}

		return idToIPBucket.Delete([]byte(e.ClientID))
	})
}

// Update implements storage.LeaseStorage
func (s *Storage) Update(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		ipLeaseBucket, _, err := openOrCreateBuckets(tx)
		if err != nil {
			return err
		}
		if err := assertUniqueIP(ipLeaseBucket, ip, clientID); err != nil {
			return err
		}
		if ipLeaseBucket.Get([]byte(ip)) == nil {
			return &storage.ErrIPNotFound{IP: ip}
		}

		e := entry{
			ClientID: clientID,
			Expires:  expiration.Unix(),
			Leased:   leased,
		}
		blob, err := json.Marshal(e)
		if err != nil {
			return err
		}

		return ipLeaseBucket.Put([]byte(ip), blob)
	})
}

// FindByIP implements storage.LeaseStorage
func (s *Storage) FindByIP(ctx context.Context, ip net.IP) (string, bool, time.Time, error) {
	var e entry
	err := s.db.View(func(tx *bbolt.Tx) error {
		ipLeaseBucket := tx.Bucket(ipLeaseBucketKey)
		if ipLeaseBucket == nil {
			// not found because the bucket hasn't even been created yet
			return &storage.ErrIPNotFound{IP: ip}
		}

		blob := ipLeaseBucket.Get([]byte(ip))
		if blob == nil {
			return &storage.ErrIPNotFound{IP: ip}
		}

		return json.Unmarshal(blob, &e)
	})

	return e.ClientID, e.Leased, time.Unix(e.Expires, 0), err
}

// FindByID implements storage.LeaseStorage
func (s *Storage) FindByID(ctx context.Context, clientID string) (net.IP, bool, time.Time, error) {
	var e entry
	var ip net.IP

	err := s.db.View(func(tx *bbolt.Tx) error {
		idToIPBucket := tx.Bucket(idToIPBucketKey)
		ipBucket := tx.Bucket(ipLeaseBucketKey)
		if idToIPBucket == nil || ipBucket == nil {
			// not found because the bucket hasn't even been created yet
			// TODO(ppacher): create ErrClientNotFound in lease/storage
			return &storage.ErrIPNotFound{}
		}

		ip = net.IP(idToIPBucket.Get([]byte(clientID)))
		if ip == nil {
			// TODO(ppacher): see above
			return &storage.ErrIPNotFound{}
		}

		blob := ipBucket.Get([]byte(ip))
		if blob == nil {
			return fmt.Errorf("database inconsistency detected. IP lease entry does not exist for IP %s (clientID:%s)", ip, clientID)
		}

		return json.Unmarshal(blob, &e)
	})

	return ip, e.Leased, time.Unix(e.Expires, 0), err
}

// ListIPs returns a list of all IPs and implements storage.LeaseStorage
func (s *Storage) ListIPs(ctx context.Context) ([]net.IP, error) {
	var ips []net.IP
	return ips, s.db.View(func(tx *bbolt.Tx) error {
		ipBucket := tx.Bucket(ipLeaseBucketKey)
		if ipBucket == nil {
			return nil
		}

		cursor := ipBucket.Cursor()
		key, _ := cursor.First()
		for key != nil {
			ips = append(ips, net.IP(key))
			key, _ = cursor.Next()
		}

		return nil
	})
}

// ListIDs returns a list of all IDs and implements storage.LeaseStorage
func (s *Storage) ListIDs(ctx context.Context) ([]string, error) {
	var ids []string
	return ids, s.db.View(func(tx *bbolt.Tx) error {
		idToIPBucket := tx.Bucket(idToIPBucketKey)
		if idToIPBucket == nil {
			return nil
		}

		cursor := idToIPBucket.Cursor()
		key, _ := cursor.First()
		for key != nil {
			ids = append(ids, string(key))
			key, _ = cursor.Next()
		}
		return nil
	})
}

func openOrCreateBuckets(tx *bbolt.Tx) (ipLeaseBucket *bbolt.Bucket, idToIPBucket *bbolt.Bucket, err error) {
	ipLeaseBucket, err = tx.CreateBucketIfNotExists(ipLeaseBucketKey)
	if err != nil {
		return
	}

	idToIPBucket, err = tx.CreateBucketIfNotExists(idToIPBucketKey)
	if err != nil {
		return
	}

	return ipLeaseBucket, idToIPBucket, nil
}

func assertUniqueClient(bucket *bbolt.Bucket, clientID string, ip net.IP) error {
	existingIP := bucket.Get([]byte(clientID))
	if existingIP != nil && !net.IP(existingIP).Equal(ip) {
		return &storage.ErrDuplicateClientID{
			ClientID: clientID,
			IP:       net.IP(existingIP).To4(),
		}
	}
	return nil
}

func assertUniqueIP(bucket *bbolt.Bucket, ip net.IP, clientID string) error {
	existingEntry := bucket.Get([]byte(ip))
	if existingEntry == nil {
		return nil
	}

	var e entry
	if err := json.Unmarshal(existingEntry, &e); err != nil {
		return err
	}

	if e.ClientID != clientID {
		return &storage.ErrDuplicateIP{
			IP:       ip,
			ClientID: e.ClientID,
		}
	}

	return nil
}
