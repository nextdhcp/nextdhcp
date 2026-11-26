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

// forEachDriver runs test as a subtest for each storage driver, with a new
// storage of that driver.
func forEachDriver(t *testing.T, test func(t *testing.T, store storage.LeaseStorage)) {
	t.Helper()

	for _, driver := range []string{"memory", "bolt"} {
		t.Run(driver, func(t *testing.T) {
			test(t, openStorage(t, driver))
		})
	}
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

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {

		// before the address is static, another client leases it, and the
		// client it's assigned to leases another one
		db := storage.NewDatabase(store)
		_, err := db.Lease(ctx, staticIP, client1, time.Hour, false)
		require.NoError(t, err)
		_, err = db.Lease(ctx, otherIP, staticClient, time.Hour, false)
		require.NoError(t, err)

		// the database is opened again
		db = storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))

		// the other client may still use its lease, so neither reserving the
		// address nor removing the conflicts removes it, and the static
		// client doesn't get the address yet
		require.NoError(t, db.RemoveStaticConflicts(ctx))

		owner, _, _, err := store.FindByIP(ctx, staticIP)
		require.NoError(t, err)
		assert.Equal(t, client1.HwAddr.String(), owner)

		assert.ErrorIs(t, db.Reserve(ctx, staticIP, staticClient), lease.ErrAddressReserved)

		// other clients can neither renew, lease nor reserve the address
		_, err = db.Lease(ctx, staticIP, client1, time.Hour, true)
		assert.ErrorIs(t, err, lease.ErrAddressReserved)
		_, err = db.Lease(ctx, staticIP, client2, time.Hour, false)
		assert.ErrorIs(t, err, lease.ErrAddressReserved)
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, client2), lease.ErrAddressReserved)

		// the lease of the client whose renewal was refused is removed, so
		// that client can get another address
		_, _, _, err = store.FindByIP(ctx, staticIP)
		assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)
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
		// and the lease of the static client is stored, in place of its
		// lease of another address, which it gave up
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
		assert.True(t, leases[0].Address.Equal(staticIP))
		assert.Equal(t, staticClient.HwAddr.String(), leases[0].ID)

		owner, leased, _, err := store.FindByIP(ctx, staticIP)
		require.NoError(t, err)
		assert.Equal(t, staticClient.HwAddr.String(), owner)
		assert.True(t, leased)

		// releasing the address succeeds, and deleting the reservation
		// fails, without removing it
		assert.NoError(t, db.Release(ctx, staticIP))
		assert.ErrorIs(t, db.DeleteReservation(ctx, staticIP, nil), lease.ErrStaticReservation)
		assert.ErrorIs(t, db.DeleteReservation(ctx, staticIP, &staticClient), lease.ErrStaticReservation)
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, client2), lease.ErrAddressReserved)

		// static reservations can be repeated, but not changed
		assert.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: staticIP}))
		assert.Error(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: client2, IP: staticIP}))
		assert.Error(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.IP{10, 0, 0, 4}}))
	})
}

func TestReserveStaticInvalid(t *testing.T) {
	db := storage.NewDatabase(openStorage(t, "memory"))
	ctx := context.Background()

	client := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	err := db.ReserveStatic(ctx, lease.ReservedAddress{Client: client, IP: net.ParseIP("fd00::1")})
	assert.ErrorIs(t, err, lease.ErrInvalidAddress)
	assert.ErrorContains(t, err, "fd00::1 is not an IPv4 address")
	assert.ErrorIs(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: client}), lease.ErrInvalidAddress)

	assert.Error(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: lease.Client{ID: "client"}, IP: net.IP{10, 0, 0, 1}}))

	// static reservations don't expire
	expires := time.Now().Add(time.Hour)
	assert.Error(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: client, IP: net.IP{10, 0, 0, 1}, Expires: &expires}))
}

// TestReserveExpiredOfOtherClient checks that an IP address whose reservation
// for another client expired can be reserved again.
func TestReserveExpiredOfOtherClient(t *testing.T) {
	ctx := context.Background()
	ip := net.IP{10, 0, 0, 1}
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
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

// TestLeaseReservedAddress checks that a client that leases the address it
// reserved gets the full lease time, not what remains of its reservation.
func TestLeaseReservedAddress(t *testing.T) {
	ctx := context.Background()
	ip := net.IP{10, 0, 0, 1}
	client := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		db := storage.NewDatabase(store)

		// like the range plugin, which reserves the address it offers and
		// leases it when the client requests it
		require.NoError(t, db.Reserve(ctx, ip, client))
		leaseTime, err := db.Lease(ctx, ip, client, time.Hour, false)
		require.NoError(t, err)
		assert.Equal(t, time.Hour, leaseTime)

		_, leased, expiration, err := store.FindByIP(ctx, ip)
		require.NoError(t, err)
		assert.True(t, leased)
		assert.WithinDuration(t, time.Now().Add(time.Hour), expiration, 5*time.Second)
	})
}

// TestLeaseAlmostExpired checks that a client that keeps its lease gets the
// full lease time if less than a second of the lease remains, as lease times
// are sent in whole seconds, while a client with more time left keeps it.
func TestLeaseAlmostExpired(t *testing.T) {
	ctx := context.Background()
	client := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 1}, client.HwAddr.String(), true, time.Now().Add(500*time.Millisecond)))
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 2}, other.HwAddr.String(), true, time.Now().Add(10*time.Minute)))

		db := storage.NewDatabase(store)

		leaseTime, err := db.Lease(ctx, net.IP{10, 0, 0, 1}, client, time.Hour, false)
		require.NoError(t, err)
		assert.Equal(t, time.Hour, leaseTime)

		_, _, expiration, err := store.FindByIP(ctx, net.IP{10, 0, 0, 1})
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(time.Hour), expiration, 5*time.Second)

		leaseTime, err = db.Lease(ctx, net.IP{10, 0, 0, 2}, other, time.Hour, false)
		require.NoError(t, err)
		assert.InDelta(t, 10*time.Minute, leaseTime, float64(5*time.Second))
	})
}

// TestReleaseStaticConflict checks that releasing a statically reserved IP
// address removes a stored lease of it that RemoveStaticConflicts didn't
// remove, for example because it failed, but keeps the static reservation.
func TestReleaseStaticConflict(t *testing.T) {
	ctx := context.Background()
	staticIP := net.IP{10, 0, 0, 1}
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, staticIP, other.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))

		require.NoError(t, db.Release(ctx, staticIP))

		_, _, _, err := store.FindByIP(ctx, staticIP)
		assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)

		// so the other client can get another address
		assert.NoError(t, db.Reserve(ctx, net.IP{10, 0, 0, 2}, other))
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, other), lease.ErrAddressReserved)

		// releasing it again still succeeds
		assert.NoError(t, db.Release(ctx, staticIP))
	})
}

// TestReserveExpiredKeptForClientWithAddress checks that reserving an IP
// address whose entry of another client expired doesn't remove that entry
// if the client can't get the address anyway, as it has another one.
func TestReserveExpiredKeptForClientWithAddress(t *testing.T) {
	ctx := context.Background()
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 1}, client1.HwAddr.String(), true, time.Now().Add(-time.Minute)))
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 2}, client2.HwAddr.String(), false, time.Now().Add(time.Minute)))

		db := storage.NewDatabase(store)

		var duplicate *storage.ErrDuplicateClientID
		assert.ErrorAs(t, db.Reserve(ctx, net.IP{10, 0, 0, 1}, client2), &duplicate)

		owner, _, _, err := store.FindByIP(ctx, net.IP{10, 0, 0, 1})
		require.NoError(t, err)
		assert.Equal(t, client1.HwAddr.String(), owner)
	})
}

// TestLeaseExpiredKeptForClientWithAddress checks that leasing an IP address
// whose entry of another client expired doesn't remove that entry if the
// client can't get the address anyway, as it has another one.
func TestLeaseExpiredKeptForClientWithAddress(t *testing.T) {
	ctx := context.Background()
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 1}, client1.HwAddr.String(), true, time.Now().Add(-time.Minute)))
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 2}, client2.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)

		var duplicate *storage.ErrDuplicateClientID
		_, err := db.Lease(ctx, net.IP{10, 0, 0, 1}, client2, time.Hour, false)
		assert.ErrorAs(t, err, &duplicate)

		owner, _, _, err := store.FindByIP(ctx, net.IP{10, 0, 0, 1})
		require.NoError(t, err)
		assert.Equal(t, client1.HwAddr.String(), owner)
	})
}

// TestStaticConflictHealed checks that when another client asks for a
// statically reserved IP address, its stored lease of the address that
// RemoveStaticConflicts didn't remove, for example because it failed, is
// removed, so that the client can get another address.
func TestStaticConflictHealed(t *testing.T) {
	ctx := context.Background()
	staticIP := net.IP{10, 0, 0, 1}
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, staticIP, other.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))

		// the other client tries to renew its lease
		_, err := db.Lease(ctx, staticIP, other, time.Hour, true)
		assert.ErrorIs(t, err, lease.ErrAddressReserved)

		_, _, _, err = store.FindByIP(ctx, staticIP)
		assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)

		// so it can get another address
		assert.NoError(t, db.Reserve(ctx, net.IP{10, 0, 0, 2}, other))
	})
}

// TestStaticConflictKeepsOwnerLease checks that removing the conflicts with a
// static reservation and releasing the address keep a stored lease of the
// address for the client it's reserved for.
func TestStaticConflictKeepsOwnerLease(t *testing.T) {
	ctx := context.Background()
	staticIP := net.IP{10, 0, 0, 1}
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, staticIP, staticClient.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))

		require.NoError(t, db.RemoveStaticConflicts(ctx))
		require.NoError(t, db.Release(ctx, staticIP))

		owner, leased, _, err := store.FindByIP(ctx, staticIP)
		require.NoError(t, err)
		assert.Equal(t, staticClient.HwAddr.String(), owner)
		assert.True(t, leased)
	})
}

// TestLeaseStatic checks that the lease of a statically reserved IP address
// for its client is stored, in place of the client's lease of another
// address, which it gave up, and of another client's lease of the address.
func TestLeaseStatic(t *testing.T) {
	ctx := context.Background()
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		// another client's lease of the address expired
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 1}, other.HwAddr.String(), true, time.Now().Add(-time.Minute)))
		require.NoError(t, store.Create(ctx, net.IP{10, 0, 0, 2}, staticClient.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))

		// the static plugin has addresses in the 16 byte form
		leaseTime, err := db.Lease(ctx, net.ParseIP("10.0.0.1"), staticClient, time.Hour, true)
		require.NoError(t, err)
		assert.Equal(t, time.Hour, leaseTime)

		// storages and DHCP messages have them in the 4 byte form
		owner, leased, expiration, err := store.FindByIP(ctx, net.IP{10, 0, 0, 1})
		require.NoError(t, err)
		assert.Equal(t, staticClient.HwAddr.String(), owner)
		assert.True(t, leased)
		assert.WithinDuration(t, time.Now().Add(time.Hour), expiration, 5*time.Second)

		_, _, _, err = store.FindByIP(ctx, net.IP{10, 0, 0, 2})
		assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)

		leases, err := db.Leases(ctx)
		require.NoError(t, err)
		require.Len(t, leases, 1)
		assert.True(t, leases[0].Address.Equal(net.IP{10, 0, 0, 1}))

		// renewing it extends it
		leaseTime, err = db.Lease(ctx, net.IP{10, 0, 0, 1}, staticClient, 2*time.Hour, true)
		require.NoError(t, err)
		assert.Equal(t, 2*time.Hour, leaseTime)

		_, _, expiration, err = store.FindByIP(ctx, net.IP{10, 0, 0, 1})
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(2*time.Hour), expiration, 5*time.Second)
	})
}

// TestStaticAddressInUse checks that a statically reserved IP address isn't
// leased to its client while another client has a lease of it that hasn't
// expired, from before the address was reserved, which that client may still
// use, until that client asks for an address.
func TestStaticAddressInUse(t *testing.T) {
	ctx := context.Background()
	staticIP := net.IP{10, 0, 0, 1}
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	client1 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}
	client2 := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, staticIP, client1.HwAddr.String(), true, time.Now().Add(time.Hour)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))
		require.NoError(t, db.RemoveStaticConflicts(ctx))

		inUse := func() {
			t.Helper()

			owner, _, _, err := store.FindByIP(ctx, staticIP)
			require.NoError(t, err)
			assert.Equal(t, client1.HwAddr.String(), owner)
		}

		// the static client doesn't get the address
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, staticClient), lease.ErrAddressReserved)
		_, err := db.Lease(ctx, staticIP, staticClient, time.Hour, true)
		assert.ErrorIs(t, err, lease.ErrAddressReserved)
		inUse()

		// a third client asking for it doesn't end the lease
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, client2), lease.ErrAddressReserved)
		inUse()

		// the client asking for it does, so the static client gets it
		assert.ErrorIs(t, db.Reserve(ctx, staticIP, client1), lease.ErrAddressReserved)
		assert.NoError(t, db.Reserve(ctx, staticIP, staticClient))
		leaseTime, err := db.Lease(ctx, staticIP, staticClient, time.Hour, true)
		assert.NoError(t, err)
		assert.Equal(t, time.Hour, leaseTime)
	})
}

// TestStaticAddressReservedForAnother checks that removing the conflicts with
// a static reservation removes another client's reservation of the address,
// which it doesn't use.
func TestStaticAddressReservedForAnother(t *testing.T) {
	ctx := context.Background()
	staticIP := net.IP{10, 0, 0, 1}
	staticClient := lease.Client{HwAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}}

	forEachDriver(t, func(t *testing.T, store storage.LeaseStorage) {
		require.NoError(t, store.Create(ctx, staticIP, other.HwAddr.String(), false, time.Now().Add(time.Minute)))

		db := storage.NewDatabase(store)
		require.NoError(t, db.ReserveStatic(ctx, lease.ReservedAddress{Client: staticClient, IP: net.ParseIP("10.0.0.1")}))
		require.NoError(t, db.RemoveStaticConflicts(ctx))

		_, _, _, err := store.FindByIP(ctx, staticIP)
		assert.True(t, storage.IsNotFound(err), "unexpected error %v", err)
		assert.NoError(t, db.Reserve(ctx, staticIP, staticClient))
	})
}
