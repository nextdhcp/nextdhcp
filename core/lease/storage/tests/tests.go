package tests

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	// StorageFactory should create a new storage instance
	StorageFactory func(ctx context.Context) storage.LeaseStorage

	// TeardownFunc is invoked after each test case
	TeardownFunc func(storage.LeaseStorage)
)

// Run executes a test suite to ensure storage implementations match the
// requirements
func Run(t *testing.T, factory StorageFactory, teardown TeardownFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	instance := factory(ctx)
	require.NotNil(t, instance)
	defer teardown(instance)

	count := func() int {
		allIPs, err := instance.ListIPs(ctx)
		allIDs, err2 := instance.ListIDs(ctx)
		require.NoError(t, err)
		require.NoError(t, err2)
		assert.Equal(t, len(allIPs), len(allIDs))

		return len(allIPs)
	}

	t.Run("Create", func(t *testing.T) {
		// adding a unique IP/clientID must work, implementation should NOT
		// check the expiration
		alreadyExpired := time.Now().Add(-time.Minute)
		err := instance.Create(ctx, net.IP{10, 0, 0, 1}, "client-1", true, alreadyExpired)
		assert.NoError(t, err, "adding a uniqe IP/client pair must work")
		assert.Equal(t, 1, count())

		err = instance.Create(ctx, net.IP{10, 0, 0, 1}, "client-1", true, alreadyExpired)
		assert.Error(t, err, "must not allow re-creating of an existing pair")
		assert.Equal(t, 1, count())

		// reusing the IP is not allowed
		err = instance.Create(ctx, net.IP{10, 0, 0, 1}, "client-2", false, alreadyExpired)
		assert.Error(t, err, "IPs must not be allowed to be re-used")
		eid, ok := err.(*storage.ErrDuplicateIP)
		assert.True(t, ok, "invalid error type returned")
		assert.True(t, eid.IP.Equal(net.IP{10, 0, 0, 1}))
		assert.Equal(t, 1, count())

		// reusing the client is not allowed
		err = instance.Create(ctx, net.IP{10, 0, 0, 2}, "client-1", false, alreadyExpired)
		assert.Error(t, err, "ClientIDs must not be allowed to be re-used")
		ecid, ok := err.(*storage.ErrDuplicateClientID)
		assert.True(t, ok, "invalid error type returned")
		assert.Equal(t, "client-1", ecid.ClientID)
		assert.Equal(t, 1, count())

		err = instance.Create(ctx, net.IP{10, 0, 0, 3}, "client-3", true, alreadyExpired)
		assert.NoError(t, err, "adding a uniqe IP/client pair must work")
		assert.Equal(t, 2, count())
	})

	t.Run("FindByIP", func(t *testing.T) {
		id, leased, expires, err := instance.FindByIP(ctx, net.IP{10, 0, 0, 1})
		assert.NoError(t, err)
		assert.Equal(t, "client-1", id)
		assert.True(t, leased)
		assert.InDelta(t, time.Now().Add(-time.Minute).Unix(), expires.Unix(), 1)

		_, _, _, err = instance.FindByIP(ctx, net.IP{10, 0, 0, 2})
		assert.Error(t, err)
	})

	t.Run("FindByID", func(t *testing.T) {
		ip, leased, expires, err := instance.FindByID(ctx, "client-3")
		assert.NoError(t, err)
		assert.Equal(t, net.IP{10, 0, 0, 3}, ip.To4())
		assert.True(t, leased)
		assert.InDelta(t, time.Now().Add(-time.Minute).Unix(), expires.Unix(), 1)

		_, _, _, err = instance.FindByID(ctx, "client-2")
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		expires := time.Now().Add(time.Minute)
		err := instance.Update(ctx, net.IP{10, 0, 0, 1}, "client-1", false, expires)
		assert.NoError(t, err)
		assert.Equal(t, 2, count())

		_, leased, expiresAt, err := instance.FindByID(ctx, "client-1")
		assert.NoError(t, err)
		assert.False(t, leased)
		assert.Equal(t, expires.Unix(), expiresAt.Unix())
		assert.Equal(t, 2, count())

		// IP and clientID must match for the update
		err = instance.Update(ctx, net.IP{10, 0, 0, 1}, "client-2", false, expires)
		assert.Error(t, err)
		assert.Equal(t, 2, count())

		// IP and clientID must match for the update
		err = instance.Update(ctx, net.IP{10, 0, 0, 3}, "client-1", false, expires)
		assert.Error(t, err)
		assert.Equal(t, 2, count())

		// IP must be available
		err = instance.Update(ctx, net.IP{10, 0, 0, 2}, "client-2", false, expires)
		assert.Error(t, err)
		assert.Equal(t, 2, count())
	})

	t.Run("Delete", func(t *testing.T) {
		assert.NoError(t, instance.Delete(ctx, net.IP{10, 0, 0, 1}, "client-1"))
		assert.Equal(t, 1, count())

		assert.Error(t, instance.Delete(ctx, net.IP{10, 0, 0, 2}, "client-2"))
		assert.Equal(t, 1, count())

		assert.Error(t, instance.Delete(ctx, net.IP{10, 0, 0, 3}, "wrong-client-id"))
		assert.Equal(t, 1, count())

		assert.NoError(t, instance.Delete(ctx, net.IP{10, 0, 0, 3}, "client-3"))
		assert.Equal(t, 0, count())
	})
}
