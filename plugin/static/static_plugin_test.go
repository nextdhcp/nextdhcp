package static

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticServeDHCP(t *testing.T) {
	p, err := makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee 127.0.0.50")
	require.NoError(t, err)

	// the next handler is only called for clients without a static address
	nextCalled := false
	p.Wrap(test.HandlerFunc(func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
		nextCalled = true
		return nil
	}))

	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	t.Run("discover", func(t *testing.T) {
		req, err := dhcpv4.NewDiscovery(mac, dhcpv4.WithRequestedOptions(dhcpv4.OptionSubnetMask))
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		require.NoError(t, p.ServeDHCP(context.Background(), req, res))
		assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())
		assert.Equal(t, "255.0.0.0", net.IP(res.SubnetMask()).String())
		assert.Nil(t, req.SubnetMask(), "the request must not be modified")
	})

	t.Run("request", func(t *testing.T) {
		req, err := dhcpv4.NewDiscovery(mac,
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithRequestedOptions(dhcpv4.OptionSubnetMask),
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 50})),
		)
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		require.NoError(t, p.ServeDHCP(context.Background(), req, res))
		assert.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
		assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())
		assert.Equal(t, "255.0.0.0", net.IP(res.SubnetMask()).String())
	})

	t.Run("subnet mask not requested", func(t *testing.T) {
		// NewDiscovery requests the subnet mask by default, and so does a
		// request without a parameter request list
		req, err := dhcpv4.New(
			dhcpv4.WithHwAddr(mac),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
			dhcpv4.WithRequestedOptions(dhcpv4.OptionRouter),
		)
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		require.NoError(t, p.ServeDHCP(context.Background(), req, res))
		assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())
		assert.Nil(t, res.SubnetMask())
	})

	t.Run("request for another address", func(t *testing.T) {
		req, err := dhcpv4.NewDiscovery(mac,
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 51})),
		)
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		require.NoError(t, p.ServeDHCP(context.Background(), req, res))
		assert.Equal(t, dhcpv4.MessageTypeNak, res.MessageType())
	})

	t.Run("client without static address", func(t *testing.T) {
		req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xff})
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		require.NoError(t, p.ServeDHCP(context.Background(), req, res))
		assert.True(t, nextCalled)
		assert.True(t, res.YourIPAddr == nil || res.YourIPAddr.IsUnspecified())
	})
}

// TestStaticMACCase checks that clients are matched whatever the case of the
// configured MAC address.
func TestStaticMACCase(t *testing.T) {
	p, err := makeStaticPlugin(t, "static 00:AA:BB:CC:DD:EE 127.0.0.50")
	require.NoError(t, err)
	p.Wrap(test.ErrorHandler)

	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
	require.NoError(t, err)
	res, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	require.NoError(t, p.ServeDHCP(context.Background(), req, res))
	assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())

	// a client can't be configured twice, in different cases
	_, err = makeStaticPlugin(t, "static 00:AA:BB:CC:DD:EE 127.0.0.50\nstatic 00:aa:bb:cc:dd:ee 127.0.0.51")
	assert.Error(t, err)

	_, err = test.Provision(t, &Plugin{Addresses: map[string]net.IP{
		"00:AA:BB:CC:DD:EE": {127, 0, 0, 50},
		"00:aa:bb:cc:dd:ee": {127, 0, 0, 51},
	}})
	assert.Error(t, err)
}

// TestStaticStoresLease checks that the lease of a static address is stored,
// and expires when the client is told, also when the client requests the
// address again.
func TestStaticStoresLease(t *testing.T) {
	p, err := makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee 127.0.0.50")
	require.NoError(t, err)
	p.Wrap(test.ErrorHandler)

	ctx := context.Background()

	for _, leaseTime := range []time.Duration{30 * time.Minute, 45 * time.Minute} {
		req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 50})),
		)
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		// like the lease plugin
		res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(leaseTime))

		require.NoError(t, p.ServeDHCP(ctx, req, res))
		require.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
		assert.Equal(t, leaseTime, res.IPAddressLeaseTime(0))

		leases, err := p.Config.Database.Leases(ctx)
		require.NoError(t, err)
		require.Len(t, leases, 1)
		assert.Equal(t, "127.0.0.50", leases[0].Address.String())
		assert.Equal(t, "00:aa:bb:cc:dd:ee", leases[0].ID)
		assert.WithinDuration(t, time.Now().Add(leaseTime), leases[0].Expires, 5*time.Second)
	}
}
