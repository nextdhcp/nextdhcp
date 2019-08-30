package handler

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease/mockdb"
	"github.com/ppacher/dhcp-ng/pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_v4handler_Serve_prepareResponse(t *testing.T) {
	v4 := NewV4(Option{
		Subnets: []SubnetConfig{
			{
				IP: net.IP{10, 1, 1, 1},
				Network: net.IPNet{
					IP:   net.IP{10, 1, 1, 0},
					Mask: net.IPMask{255, 255, 255, 0},
				},
				Interface: net.Interface{
					Name: "eth0",
				},
				Database:    &mockdb.MockDatabase{},
				LeaseTime:   time.Hour,
				Middlewares: []middleware.Handler{},
			},
		},
	}).(*v4handler)

	mobj := mock.Mock{}

	getServe := func(name string) serveFunc {
		return func(_ *middleware.Context, r *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error) {
			args := mobj.MethodCalled(name, r, s)
			return args.Get(0).(*dhcpv4.DHCPv4), args.Error(1)
		}
	}

	v4.prepareDHCPv4Offer = getServe("perpareDHCPv4Offer")
	v4.prepareDHCPv4RequestReply = getServe("prepareDHCPv4RequestReply")
	v4.handleDHCPv4Release = getServe("handleDHCPv4Release")

	assert.NotNil(t, v4)

	var (
		ctx   = context.Background()
		iface = net.Interface{
			Name: "eth0",
		}
		peer   = &net.UDPAddr{IP: net.IP{10, 1, 1, 2}, Port: dhcpv4.ServerPort}
		hwPeer = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		req, _ = dhcpv4.NewDiscovery(net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	)

	mockRes, _ := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer))

	mobj.On("perpareDHCPv4Offer", req, &v4.subnets[0]).Return(mockRes, nil)

	res := v4.Serve(ctx, iface, peer, hwPeer, req)
	assert.NotNil(t, res)
	assert.Equal(t, mockRes, res)
}
