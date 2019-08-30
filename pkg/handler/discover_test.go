package handler

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease"
	"github.com/ppacher/dhcp-ng/pkg/lease/mockdb"
	"github.com/ppacher/dhcp-ng/pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mac1 = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x00}
	mac2 = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01}

	leaseClientPtrType = mock.AnythingOfType("*lease.Client")
	leaseClientType    = mock.AnythingOfType("lease.Client")
)

func getCtx(req *dhcpv4.DHCPv4) *middleware.Context {
	ctx, _ := middleware.NewContext(context.Background(), req, nil, nil, net.Interface{}, nil)
	return ctx
}

func getMockDB(init ...func(db *mockdb.MockDatabase)) (*SubnetConfig, *mockdb.MockDatabase) {
	db := &mockdb.MockDatabase{}
	defaultSubnet := &SubnetConfig{
		Database: db,
		IP:       net.IP{10, 1, 1, 254},
		Network: net.IPNet{
			IP:   net.IP{10, 1, 1, 0},
			Mask: net.IPMask{255, 255, 255, 0},
		},
		Interface: net.Interface{
			Name: "eth0",
		},
		LeaseTime: time.Hour,
		Options: map[dhcpv4.OptionCode]dhcpv4.OptionValue{
			dhcpv4.OptionRouter: dhcpv4.IPs([]net.IP{{10, 1, 1, 254}}),
		},
	}

	return defaultSubnet, db
}

func newDiscoveryRequest(hw string, modifiers ...dhcpv4.Modifier) *dhcpv4.DHCPv4 {
	addr, _ := net.ParseMAC(hw)
	req, _ := dhcpv4.NewDiscovery(addr, modifiers...)

	return req
}

func Test_prepareDHCPOffer_offer_IP(t *testing.T) {
	subnet, db := getMockDB()

	req, _ := dhcpv4.NewDiscovery(mac1)

	db.On("FindAddress", leaseClientPtrType).Return(net.IP{10, 1, 1, 1}, nil)
	db.On("Reserve", net.IP{10, 1, 1, 1}, leaseClientType).Return(nil)

	res, err := prepareDHCPv4Offer(getCtx(req), req, subnet)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	assert.Equal(t, net.IP{10, 1, 1, 1}, res.YourIPAddr)
	assert.Equal(t, net.IPMask{255, 255, 255, 0}, res.SubnetMask())
	assert.Equal(t, net.IP{10, 1, 1, 254}, res.ServerIdentifier())
	assert.Equal(t, net.IP{10, 1, 1, 254}, res.ServerIPAddr)
	assert.Equal(t, dhcpv4.MessageTypeOffer, res.MessageType())
	assert.Equal(t, time.Hour, res.IPAddressLeaseTime(0))
	assert.Len(t, res.Router(), 1)
	assert.Equal(t, res.Router()[0], net.IP{10, 1, 1, 254})
	db.AssertExpectations(t)
}

func Test_prepareDHCPOffer_ReservationFailes(t *testing.T) {
	subnet, db := getMockDB()
	req, _ := dhcpv4.NewDiscovery(mac1)

	db.On("FindAddress", leaseClientPtrType).Return(net.IP{10, 1, 1, 1}, nil)
	db.On("Reserve", net.IP{10, 1, 1, 1}, leaseClientType).Return(errors.New("some-error"))

	res, err := prepareDHCPv4Offer(getCtx(req), req, subnet)
	assert.Nil(t, res)
	assert.Error(t, err)
	assert.Equal(t, "some-error", err.Error())

	db.AssertExpectations(t)
}

func Test_preareDHCPOffer_useRequestedIP(t *testing.T) {
	subnet, db := getMockDB()
	req, _ := dhcpv4.NewDiscovery(mac1)
	req.UpdateOption(dhcpv4.OptRequestedIPAddress(net.IP{10, 1, 1, 2}))

	db.On("Reserve", net.IP{10, 1, 1, 2}, leaseClientType).Return(nil)

	res, err := prepareDHCPv4Offer(getCtx(req), req, subnet)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, net.IP{10, 1, 1, 2}, res.YourIPAddr)

	db.AssertExpectations(t)
}

func Test_preareDHCPOffer_fallback_from_RequestedIP(t *testing.T) {
	subnet, db := getMockDB()
	req, _ := dhcpv4.NewDiscovery(mac1)
	req.UpdateOption(dhcpv4.OptRequestedIPAddress(net.IP{10, 1, 1, 2}))

	db.On("Reserve", net.IP{10, 1, 1, 2}, leaseClientType).Return(lease.ErrNoIPAvailable)
	db.On("FindAddress", leaseClientPtrType).Return(net.IP{10, 1, 1, 1}, nil)

	db.On("Reserve", net.IP{10, 1, 1, 1}, leaseClientType).Return(nil)

	res, err := prepareDHCPv4Offer(getCtx(req), req, subnet)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, net.IP{10, 1, 1, 1}, res.YourIPAddr)

	db.AssertExpectations(t)
}
