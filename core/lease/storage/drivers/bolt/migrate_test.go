package bolt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func copyFile(srcFile string) (*os.File, error) {
	file, err := os.CreateTemp("", "storage-*.db")
	if err != nil {
		return nil, err
	}

	src, err := os.Open(srcFile)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	_, err = io.Copy(file, src)
	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func TestMigrateDb(t *testing.T) {
	f, err := copyFile("testdata/leases.db")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	db, err := bbolt.Open("testdata/leases.db", 0o600, &bbolt.Options{
		OpenFile: func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			return f, nil
		},
	})
	require.NoError(t, err)

	err = migrateDatabase(db)
	assert.NoError(t, err)

	s := &Storage{db: db}
	ips, err := s.ListIPs(context.Background())
	assert.NoError(t, err)
	ids, err := s.ListIDs(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, len(ips), len(ids))

	// we should not have migrated any expired reservation
	countLeased := 0
	for _, ip := range ips {
		cli, leased, expired, err := s.FindByIP(context.Background(), ip)
		assert.NoError(t, err)
		assert.NotEmpty(t, cli)

		if expired.Before(time.Now()) {
			assert.True(t, leased)
		}

		if leased {
			countLeased++
		}

		// we must also find the same entry via it's ID
		refIP, refLeased, refExpired, refErr := s.FindByID(context.Background(), cli)
		require.NoError(t, refErr, "ip=%q cli=%q leased=%v expired=%q", ip, cli, leased, expired)
		assert.Equal(t, leased, refLeased)
		assert.Equal(t, expired.Unix(), refExpired.Unix())
		assert.True(t, ip.Equal(refIP))
	}

	assert.Equal(t, 7, countLeased)
}

// withDatabase opens file with the bolt driver, like a server does, calls fn
// with the storage, and closes it.
func withDatabase(t *testing.T, file string, fn func(s *Storage)) {
	t.Helper()

	store, err := storageFactory(map[string][]string{"file": {file}})
	require.NoError(t, err)

	s := store.(*Storage)
	defer func() { assert.NoError(t, s.Close()) }()

	fn(s)
}

// entries returns the entries of s, by their IP address.
func entries(t *testing.T, s *Storage) map[string]string {
	t.Helper()
	ctx := context.Background()

	ips, err := s.ListIPs(ctx)
	require.NoError(t, err)

	m := make(map[string]string, len(ips))
	for _, ip := range ips {
		cli, leased, expires, err := s.FindByIP(ctx, ip)
		require.NoError(t, err)

		m[ip.String()] = fmt.Sprintf("client=%s leased=%v expires=%d", cli, leased, expires.Unix())
	}

	return m
}

// assertConsistent checks that the IP address and the client indexes of s
// agree on each entry.
func assertConsistent(t *testing.T, s *Storage) {
	t.Helper()
	ctx := context.Background()

	ips, err := s.ListIPs(ctx)
	require.NoError(t, err)
	ids, err := s.ListIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, ids, len(ips))

	for _, ip := range ips {
		cli, leased, expires, err := s.FindByIP(ctx, ip)
		require.NoError(t, err)

		clientIP, clientLeased, clientExpires, err := s.FindByID(ctx, cli)
		if assert.NoError(t, err, "client %s of IP %s", cli, ip) {
			assert.True(t, ip.Equal(clientIP), "client %s of IP %s has IP %s", cli, ip, clientIP)
			assert.Equal(t, leased, clientLeased)
			assert.Equal(t, expires.Unix(), clientExpires.Unix())
		}
	}
}

// migrated checks that the database of s has the current schema version and
// no buckets of the first schema.
func migrated(t *testing.T, s *Storage) {
	t.Helper()

	require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
		var version string
		if b := tx.Bucket(schemaVersionBucket); b != nil {
			version = string(b.Get(schemaVersionKey))
		}

		assert.Equal(t, SchemaVersion, version)
		assert.Nil(t, tx.Bucket(v0LeasesBucket))
		assert.Nil(t, tx.Bucket(v0ReservationBucket))

		return nil
	}))
}

// putV0 stores an entry of the first schema of ip for the client with the
// hardware address mac.
func putV0(tx *bbolt.Tx, bucket []byte, ip net.IP, mac string, expires time.Time) error {
	b, err := tx.CreateBucketIfNotExists(bucket)
	if err != nil {
		return err
	}

	blob, err := json.Marshal(v0Binding{Expires: expires.Unix(), Created: time.Now().Unix(), MAC: mac})
	if err != nil {
		return err
	}

	return b.Put(ip.To4(), blob)
}

// TestOpenMigratesDatabase checks that the driver migrates a database of the
// first schema when it opens it, so its leases are kept, and that opening it
// again changes nothing.
func TestOpenMigratesDatabase(t *testing.T) {
	f, err := copyFile("testdata/leases.db")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	defer os.Remove(f.Name())

	var first map[string]string
	withDatabase(t, f.Name(), func(s *Storage) {
		first = entries(t, s)
		assert.NotEmpty(t, first)
		assertConsistent(t, s)
		migrated(t, s)

		leased := 0
		for _, e := range first {
			if strings.Contains(e, "leased=true") {
				leased++
			}
		}
		assert.Equal(t, 7, leased)
	})

	withDatabase(t, f.Name(), func(s *Storage) {
		assert.Equal(t, first, entries(t, s))
	})
}

// TestOpenKeepsCurrentDatabase checks that opening a database of the current
// schema without schema version, which NextDHCP stored before it migrated
// databases, keeps its entries.
func TestOpenKeepsCurrentDatabase(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "leases.db")
	expires := time.Now().Add(time.Hour)

	db, err := bbolt.Open(file, 0o600, nil)
	require.NoError(t, err)

	s := &Storage{db: db}
	require.NoError(t, s.Create(ctx, net.IP{10, 0, 0, 1}, "00:00:00:00:00:01", true, expires))
	require.NoError(t, s.Create(ctx, net.IP{10, 0, 0, 2}, "00:00:00:00:00:02", false, expires.Add(-30*time.Minute)))
	before := entries(t, s)
	require.NoError(t, db.Close())

	withDatabase(t, file, func(s *Storage) {
		assert.Equal(t, before, entries(t, s))
		assertConsistent(t, s)
		migrated(t, s)
	})
}

// TestOpenMigratesWithoutReplacingEntries checks that migrating a database
// that also holds entries of the current schema, which a NextDHCP that didn't
// migrate databases stored next to the old ones, keeps those entries, and
// only adds the old entries whose IP address and client have no entry yet.
func TestOpenMigratesWithoutReplacingEntries(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "leases.db")
	expires := time.Now().Add(time.Hour)

	const (
		clientX lease.ClientID = "00:00:00:00:00:01"
		clientY lease.ClientID = "00:00:00:00:00:02"
		clientZ lease.ClientID = "00:00:00:00:00:03"
		clientW lease.ClientID = "00:00:00:00:00:04"
	)

	db, err := bbolt.Open(file, 0o600, nil)
	require.NoError(t, err)

	s := &Storage{db: db}
	require.NoError(t, s.Create(ctx, net.IP{10, 0, 0, 1}, clientX, true, expires))
	require.NoError(t, s.Create(ctx, net.IP{10, 0, 0, 2}, clientY, true, expires))
	before := entries(t, s)

	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return errors.Join(
			// another client's lease of an address that has an entry
			putV0(tx, v0LeasesBucket, net.IP{10, 0, 0, 1}, string(clientZ), expires),
			// another lease of a client that has an entry
			putV0(tx, v0LeasesBucket, net.IP{10, 0, 0, 3}, string(clientX), expires),
			// a lease without conflict
			putV0(tx, v0LeasesBucket, net.IP{10, 0, 0, 4}, string(clientW), expires),
			// a lease without client
			putV0(tx, v0LeasesBucket, net.IP{10, 0, 0, 5}, "", expires),
			// a reservation of the client of the lease without conflict
			putV0(tx, v0ReservationBucket, net.IP{10, 0, 0, 6}, string(clientW), expires),
		)
	}))
	require.NoError(t, db.Close())

	withDatabase(t, file, func(s *Storage) {
		after := entries(t, s)
		for ip, e := range before {
			assert.Equal(t, e, after[ip], ip)
		}

		cli, leased, _, err := s.FindByIP(ctx, net.IP{10, 0, 0, 4})
		if assert.NoError(t, err) {
			assert.Equal(t, clientW, cli)
			assert.True(t, leased)
		}

		assert.Len(t, after, 3)
		assertConsistent(t, s)
		migrated(t, s)
	})
}
