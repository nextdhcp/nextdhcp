package mockdb

import (
	"context"
	"net"
	"time"

	"github.com/ppacher/dhcp-ng/pkg/lease"
	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
	"github.com/stretchr/testify/mock"
)

// MockDatabase is used to simplify testing code that requires a lease.Database
type MockDatabase struct {
	mock.Mock
}

func (m *MockDatabase) Leases(context.Context) ([]lease.Lease, error) {
	args := m.Called()

	return args.Get(0).([]lease.Lease), args.Error(1)
}

func (m *MockDatabase) ReservedAddresses(context.Context) ([]lease.ReservedAddress, error) {
	args := m.Called()

	return args.Get(0).([]lease.ReservedAddress), args.Error(1)
}

func (m *MockDatabase) FindAddress(_ context.Context, cli *lease.Client) (net.IP, error) {
	args := m.Called(cli)
	return args.Get(0).(net.IP), args.Error(1)
}

func (m *MockDatabase) Lease(_ context.Context, ip net.IP, cli lease.Client, leaseTime time.Duration, renew bool) (time.Duration, error) {
	args := m.Called(ip, cli, leaseTime, renew)
	return args.Get(0).(time.Duration), args.Error(1)
}

func (m *MockDatabase) Reserve(_ context.Context, ip net.IP, cli lease.Client) error {
	return m.Called(ip, cli).Error(0)
}

func (m *MockDatabase) Release(_ context.Context, ip net.IP) error {
	return m.Called(ip).Error(0)
}

func (m *MockDatabase) DeleteReservation(_ context.Context, ip net.IP, cli *lease.Client) error {
	return m.Called(ip, cli).Error(0)
}

func (m *MockDatabase) ReleaseClient(_ context.Context, cli *lease.Client) error {
	return m.Called(cli).Error(0)
}

func (m *MockDatabase) AddRange(ranges ...*iprange.IPRange) error {
	return m.Called(ranges).Error(0)
}

func (m *MockDatabase) DeleteRange(ranges ...*iprange.IPRange) error {
	return m.Called(ranges).Error(0)
}

// compile time check
var _ lease.Database = &MockDatabase{}
