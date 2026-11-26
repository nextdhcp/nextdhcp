package storage_test

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/nextdhcp/nextdhcp/core/lease/storage/drivers"
)

// openStorage opens a storage with driver, and closes it when the test ends.
func openStorage(t *testing.T, driver string) storage.LeaseStorage {
	t.Helper()

	var options map[string][]string
	if driver == "bolt" {
		options = map[string][]string{"file": {filepath.Join(t.TempDir(), "leases.db")}}
	}

	store, err := storage.Open(driver, options)
	require.NoError(t, err)

	if c, ok := store.(io.Closer); ok {
		t.Cleanup(func() { assert.NoError(t, c.Close()) })
	}

	return store
}

func TestReserveStatic(t *testing.T) {
	ctx := context.Background()

	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	// DHCP messages have addresses in their 4 byte form, while static
	// addresses are configured in the 16 byte form net.ParseIP returns
	staticIP := net.IP{10, 0, 0, 1}
	otherIP := net.IP{10, 0, 0, 2}

	for _, driver := range []string{"memory", "bolt"} {
		t.Run(driver, func(t *testing.T) {
			store := openStorage(t, driver)

			// before the address is static, another client leases it, and the
			// client it's assigned to leases another one
			db := storage.NewDatabase(store)
			_, err := db.Lease(ctx, staticIP, client1, time.Hour, false)
			require.NoError(t, err)
			_, err = db.Lease(ctx, otherIP, staticClient, time.Hour, false)
			require.NoError(t, err)

			// the database is opened again
			db = storage.NewDatabase(store)
			require.NoError(t, db.ReserveStatic(ctx, net.ParseIP("10.0.0.1"), staticClient))

			// other clients can neither renew, lease nor reserve the address
			_, err = db.Lease(ctx, staticIP, client1, time.Hour, true)
			assert.ErrorIs(t, err, lease.ErrAddressReserved)
			_, err = db.Lease(ctx, staticIP, client2, time.Hour, false)
			assert.ErrorIs(t, err, lease.ErrAddressReserved)
			assert.ErrorIs(t, db.Reserve(ctx, staticIP, client2), lease.ErrAddressReserved)

			// the lease of the other client is only removed with the conflicts
			owner, _, _, err := store.FindByIP(ctx, staticIP)
			require.NoError(t, err)
			assert.Equal(t, client1.HwAddr.String(), owner)

			require.NoError(t, db.RemoveStaticConflicts(ctx))

			_, _, _, err = store.FindByIP(ctx, staticIP)
			assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)

			// so that client can get another address
			assert.NoError(t, db.Reserve(ctx, net.IP{10, 0, 0, 3}, client1))

			// the static client uses its lease of another address until it
			// renews it, so other clients don't get that address before the
			// lease expires
			assert.ErrorIs(t, db.Reserve(ctx, otherIP, client2), lease.ErrAddressReserved)
			_, err = db.Lease(ctx, otherIP, client2, time.Hour, false)
			assert.ErrorIs(t, err, lease.ErrAddressReserved)

			// the static client can lease and reserve its address
			assert.NoError(t, db.Reserve(ctx, staticIP, staticClient))
			leaseTime, err := db.Lease(ctx, staticIP, staticClient, time.Hour, false)
			assert.NoError(t, err)
			assert.Equal(t, time.Hour, leaseTime)

			// static reservations are listed once, without expiration time,
			// and leases of them aren't stored
			reserved, err := db.ReservedAddresses(ctx)
			require.NoError(t, err)

			var found []lease.ReservedAddress
			for _, r := range reserved {
				if r.IP.Equal(staticIP) {
					found = append(found, r)
				}
			}
			require.Len(t, found, 1)
			assert.Nil(t, found[0].Expires)
			assert.Equal(t, staticClient.HwAddr.String(), found[0].HwAddr.String())

			leases, err := db.Leases(ctx)
			require.NoError(t, err)
			require.Len(t, leases, 1)
			assert.True(t, leases[0].Address.Equal(otherIP))
			assert.Equal(t, staticClient.HwAddr.String(), leases[0].ID)

			_, _, _, err = store.FindByIP(ctx, staticIP)
			assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)

			// releasing or deleting the reservation doesn't remove it
			assert.NoError(t, db.Release(ctx, staticIP))
			_ = db.DeleteReservation(ctx, staticIP, nil)
			assert.ErrorIs(t, db.Reserve(ctx, staticIP, client2), lease.ErrAddressReserved)

			// static reservations can be repeated, but not changed
			assert.NoError(t, db.ReserveStatic(ctx, staticIP, staticClient))
			assert.Error(t, db.ReserveStatic(ctx, staticIP, client2))
			assert.Error(t, db.ReserveStatic(ctx, net.IP{10, 0, 0, 4}, staticClient))
		})
	}
}

func TestReserveStaticInvalid(t *testing.T) {
	db := storage.NewDatabase(openStorage(t, "memory"))
	ctx := context.Background()

	client := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	assert.ErrorIs(t, db.ReserveStatic(ctx, net.ParseIP("fd00::1"), client), lease.ErrInvalidAddress)
	assert.ErrorIs(t, db.ReserveStatic(ctx, nil, client), lease.ErrInvalidAddress)

	assert.Error(t, db.ReserveStatic(ctx, net.IP{10, 0, 0, 1}, lease.Client{ID: "client"}))
}

// TestReserveExpiredOfOtherClient checks that an IP address whose reservation
// for another client expired can be reserved again.
func TestReserveExpiredOfOtherClient(t *testing.T) {
	ctx := context.Background()
	ip := net.IP{10, 0, 0, 1}
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	for _, driver := range []string{"memory", "bolt"} {
		t.Run(driver, func(t *testing.T) {
			store := openStorage(t, driver)
			require.NoError(t, store.Create(ctx, ip, client1.HwAddr.String(), false, time.Now().Add(-time.Minute)))

			db := storage.NewDatabase(store)
			require.NoError(t, db.Reserve(ctx, ip, client2))

			owner, leased, _, err := store.FindByIP(ctx, ip)
			require.NoError(t, err)
			assert.Equal(t, client2.HwAddr.String(), owner)
			assert.False(t, leased)

			// the reservation of client2 hasn't expired yet
			assert.ErrorIs(t, db.Reserve(ctx, ip, client1), lease.ErrAddressReserved)
		})
	}
}
