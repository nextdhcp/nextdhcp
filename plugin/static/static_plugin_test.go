package static

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
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
		assert.Equal(t, lease.ClientID("00:aa:bb:cc:dd:ee"), leases[0].ID)
		assert.WithinDuration(t, time.Now().Add(leaseTime), leases[0].Expires, 5*time.Second)
	}
}

// TestStaticAddressInUse checks that a client is served by the next handler
// while its static address is still leased to another client, and gets its
// static address once that lease ends.
func TestStaticAddressInUse(t *testing.T) {
	ctx := context.Background()
	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	other := lease.NewClient(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xff})

	// another client leased the address before it was configured
	srv := test.NewServer(t)
	_, err := srv.Database.Lease(ctx, net.IP{127, 0, 0, 50}, other, time.Hour, false)
	require.NoError(t, err)

	p := &Plugin{Addresses: map[string]net.IP{mac.String(): {127, 0, 0, 50}}}
	require.NoError(t, p.Provision(dhcpserver.WithServer(test.NewContext(t), srv)))

	nextCalled := 0
	p.Wrap(test.HandlerFunc(func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
		nextCalled++
		return nil
	}))

	serve := func(modifiers ...dhcpv4.Modifier) *dhcpv4.DHCPv4 {
		t.Helper()

		req, err := dhcpv4.NewDiscovery(mac, modifiers...)
		require.NoError(t, err)
		res, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)
		require.NoError(t, p.ServeDHCP(ctx, req, res))

		return res
	}

	discover := []dhcpv4.Modifier{}
	requestStatic := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 50})),
	}
	requestOther := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 100})),
	}

	for i, modifiers := range [][]dhcpv4.Modifier{discover, requestStatic, requestOther} {
		res := serve(modifiers...)
		assert.Equal(t, i+1, nextCalled)
		assert.True(t, res.YourIPAddr == nil || res.YourIPAddr.IsUnspecified())
	}

	// the other client is refused the address, which ends its lease
	_, err = srv.Database.Lease(ctx, net.IP{127, 0, 0, 50}, other, time.Hour, true)
	assert.ErrorIs(t, err, lease.ErrAddressReserved)

	// so the client gets its static address, and is refused another one
	res := serve(discover...)
	assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())

	res = serve(requestOther...)
	assert.Equal(t, dhcpv4.MessageTypeNak, res.MessageType())

	res = serve(requestStatic...)
	assert.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
	assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())

	assert.Equal(t, 3, nextCalled)
}

// failingRelease is a lease database whose Release fails.
type failingRelease struct {
	*storage.Database
}

func (failingRelease) Release(context.Context, net.IP, *lease.Client) error {
	return assert.AnError
}

// TestStaticAddressHandedOver checks that a client gets its static address
// while it's still leased to another client if no device uses the address,
// which ends the other client's lease, and is served by the next handler
// otherwise.
func TestStaticAddressHandedOver(t *testing.T) {
	ctx := context.Background()
	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	other := lease.NewClient(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xff})
	static := net.IP{127, 0, 0, 50}

	requests := []struct {
		name      string
		modifiers []dhcpv4.Modifier
		served    func(t *testing.T, res *dhcpv4.DHCPv4)
		leased    bool // whether the address is leased to the client
	}{
		{
			name: "discover",
			served: func(t *testing.T, res *dhcpv4.DHCPv4) {
				assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())
			},
		},
		{
			name: "request",
			modifiers: []dhcpv4.Modifier{
				dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
				dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(static)),
			},
			served: func(t *testing.T, res *dhcpv4.DHCPv4) {
				assert.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
				assert.Equal(t, "127.0.0.50", res.YourIPAddr.String())
			},
			leased: true,
		},
		{
			name: "request for another address",
			modifiers: []dhcpv4.Modifier{
				dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
				dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 100})),
			},
			served: func(t *testing.T, res *dhcpv4.DHCPv4) {
				// so that the client asks for its static address
				assert.Equal(t, dhcpv4.MessageTypeNak, res.MessageType())
			},
		},
	}

	probes := []struct {
		name           string
		used           bool
		err            error
		failingRelease bool
		handedOver     bool
	}{
		{name: "no device", handedOver: true},
		{name: "device", used: true},
		{name: "probe failed", err: assert.AnError},
		{name: "release failed", failingRelease: true},
	}

	for _, request := range requests {
		for _, probe := range probes {
			t.Run(request.name+"/"+probe.name, func(t *testing.T) {
				// another client leased the address before it was assigned
				srv := test.NewServer(t)
				_, err := srv.Database.Lease(ctx, static, other, time.Hour, false)
				require.NoError(t, err)

				if probe.failingRelease {
					srv.Database = failingRelease{srv.Database.(*storage.Database)}
				}

				var asked []string
				srv.ProbeAddress = func(_ context.Context, ip net.IP) (bool, error) {
					asked = append(asked, ip.String())
					return probe.used, probe.err
				}

				p := &Plugin{Addresses: map[string]net.IP{mac.String(): static}}
				require.NoError(t, p.Provision(dhcpserver.WithServer(test.NewContext(t), srv)))

				nextCalled := false
				p.Wrap(test.HandlerFunc(func(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
					nextCalled = true
					return nil
				}))

				req, err := dhcpv4.NewDiscovery(mac, request.modifiers...)
				require.NoError(t, err)
				res, err := dhcpv4.NewReplyFromRequest(req)
				require.NoError(t, err)
				require.NoError(t, p.ServeDHCP(ctx, req, res))

				assert.Equal(t, []string{"127.0.0.50"}, asked)

				leases, err := srv.Database.Leases(ctx)
				require.NoError(t, err)
				holders := make([]lease.ClientID, 0, len(leases))
				for _, l := range leases {
					if l.Address.Equal(static) {
						holders = append(holders, l.ID)
					}
				}

				if !probe.handedOver {
					assert.True(t, nextCalled)
					assert.True(t, res.YourIPAddr == nil || res.YourIPAddr.IsUnspecified())
					assert.Equal(t, []lease.ClientID{other.ID}, holders)
					return
				}

				assert.False(t, nextCalled)
				request.served(t, res)

				if request.leased {
					assert.Equal(t, []lease.ClientID{lease.HardwareAddrID(mac)}, holders)
				} else {
					assert.Empty(t, holders)
				}
			})
		}
	}
}
