package storage

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLeaseStorage struct {
	findByIPClientID string
	findByIPLeased   bool
	findByIPErr      error

	deleteCalls    int
	deleteIP       net.IP
	deleteClientID string
	deleteErr      error
}

func (f *fakeLeaseStorage) Create(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	return errors.New("not implemented")
}

func (f *fakeLeaseStorage) Delete(ctx context.Context, ip net.IP, clientID string) error {
	f.deleteCalls++
	f.deleteIP = append(net.IP{}, ip...)
	f.deleteClientID = clientID
	return f.deleteErr
}

func (f *fakeLeaseStorage) Update(ctx context.Context, ip net.IP, clientID string, leased bool, expiration time.Time) error {
	return errors.New("not implemented")
}

func (f *fakeLeaseStorage) FindByIP(ctx context.Context, ip net.IP) (string, bool, time.Time, error) {
	return f.findByIPClientID, f.findByIPLeased, time.Time{}, f.findByIPErr
}

func (f *fakeLeaseStorage) FindByID(ctx context.Context, clientID string) (net.IP, bool, time.Time, error) {
	return nil, false, time.Time{}, errors.New("not implemented")
}

func (f *fakeLeaseStorage) ListIPs(ctx context.Context) ([]net.IP, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLeaseStorage) ListIDs(ctx context.Context) ([]string, error) {
	return nil, errors.New("not implemented")
}

func TestDeleteReservationRemovesUnleasedEntries(t *testing.T) {
	store := &fakeLeaseStorage{
		findByIPClientID: "client-1",
		findByIPLeased:   false,
	}
	db := NewDatabase(store)

	ip := net.IP{10, 0, 0, 1}
	err := db.DeleteReservation(context.Background(), ip, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, store.deleteCalls)
	assert.True(t, ip.Equal(store.deleteIP))
	assert.Empty(t, store.deleteClientID)
}

func TestDeleteReservationProtectsLeasedEntries(t *testing.T) {
	store := &fakeLeaseStorage{
		findByIPClientID: "client-1",
		findByIPLeased:   true,
	}
	db := NewDatabase(store)

	ip := net.IP{10, 0, 0, 1}
	err := db.DeleteReservation(context.Background(), ip, nil)
	require.EqualError(t, err, "reservation not found")
	assert.Equal(t, 0, store.deleteCalls)
}
