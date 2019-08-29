package handler

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease/mockdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	ctx = context.Background()

	mac1 = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x00}
	mac2 = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01}
)

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

	db.On("FindAddress", mock.AnythingOfType("*lease.Client")).Return(net.IP{10, 1, 1, 1}, nil)
	db.On("Reserve", net.IP{10, 1, 1, 1}, mock.AnythingOfType("lease.Client")).Return(nil)

	res, err := prepareDHCPv4Offer(ctx, req, subnet)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	assert.Equal(t, net.IP{10, 1, 1, 1}, res.YourIPAddr)
	assert.Equal(t, net.IPMask{255, 255, 255, 0}, res.SubnetMask())
	assert.Equal(t, net.IP{10, 1, 1, 254}, res.ServerIdentifier())
	assert.Equal(t, net.IP{10, 1, 1, 254}, res.ServerIPAddr)
	assert.Equal(t, dhcpv4.MessageTypeOffer, res.MessageType())
	assert.Equal(t, time.Hour, res.IPAddressLeaseTime(0))
}
