package mockdb

import (
	"context"
	"net"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/stretchr/testify/mock"
)

// MockDatabase is used to simplify testing code that requires a lease.Database
type MockDatabase struct {
	mock.Mock
}

// Leases implements the lease.Database interface
func (m *MockDatabase) Leases(context.Context) ([]lease.Lease, error) {
	args := m.Called()

	return args.Get(0).([]lease.Lease), args.Error(1)
}

// ReservedAddresses implements the lease.Database interface
func (m *MockDatabase) ReservedAddresses(context.Context) ([]lease.ReservedAddress, error) {
	args := m.Called()

	return args.Get(0).([]lease.ReservedAddress), args.Error(1)
}

// Lease implements the lease.Database interface
func (m *MockDatabase) Lease(_ context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	args := m.Called(ip, cli, leaseTime, renew)
	return args.Get(0).(time.Duration), args.Error(1)
}

// Reserve implements the lease.Database interface
func (m *MockDatabase) Reserve(_ context.Context, ip net.IP, cli lease.Client) error {
	return m.Called(ip, cli).Error(0)
}

// Release implements the lease.Database interface
func (m *MockDatabase) Release(_ context.Context, ip net.IP) error {
	return m.Called(ip).Error(0)
}

// DeleteReservation implements the lease.Database interface
func (m *MockDatabase) DeleteReservation(_ context.Context, ip net.IP, cli *lease.Client) error {
	return m.Called(ip, cli).Error(0)
}

// compile time check
var _ lease.Database = &MockDatabase{}
