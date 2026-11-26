package static

import (
	"context"
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeStaticPlugin sets up the plugin from input and provisions it, like a
// server does when it loads the plugin from its JSON config.
func makeStaticPlugin(t *testing.T, input string) (*Plugin, error) {
	p := new(Plugin)
	if err := p.UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)); err != nil {
		return nil, err
	}

	m, err := test.Provision(t, p)
	if err != nil {
		return nil, err
	}

	return m.(*Plugin), nil
}

func TestStaticPluginSetup(t *testing.T) {
	static, err := makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee 10.0.0.1")
	assert.NoError(t, err)
	assert.Len(t, static.Addresses, 1)
	assert.True(t, static.Addresses["00:aa:bb:cc:dd:ee"].Equal(net.IP{10, 0, 0, 1}))

	_, err = makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee")
	assert.Error(t, err)

	_, err = makeStaticPlugin(t, "static 00:aa:bb:cc 10.0.0.1")
	assert.Error(t, err)

	_, err = makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee 10.0")
	assert.Error(t, err)

	cfg := `
	static 00:aa:bb:cc:dd:ee 10.0.0.1
	static 00:bb:cc:dd:ee:ff 10.0.0.1
	`
	_, err = makeStaticPlugin(t, cfg)
	assert.Error(t, err)

	cfg = `
	static 00:aa:bb:cc:dd:ee 10.0.0.1
	static 00:aa:bb:cc:dd:ee 10.0.0.2
	`
	_, err = makeStaticPlugin(t, cfg)
	assert.Error(t, err)

	_, err = makeStaticPlugin(t, "static 00:aa:bb:cc:dd:ee fd00::1")
	assert.ErrorContains(t, err, "failed to reserve static IP fd00::1 for client 00:aa:bb:cc:dd:ee: invalid IP address: fd00::1 is not an IPv4 address")
}

// TestStaticPluginReservesAddresses checks that the addresses are reserved in
// the lease database, so other plugins don't lease them to other clients.
func TestStaticPluginReservesAddresses(t *testing.T) {
	static, err := makeStaticPlugin(t, "static 00:AA:BB:CC:DD:EE 127.0.0.50")
	require.NoError(t, err)

	ctx := context.Background()
	db := static.Config.Database

	reserved, err := db.ReservedAddresses(ctx)
	require.NoError(t, err)
	require.Len(t, reserved, 1)
	assert.Equal(t, "127.0.0.50", reserved[0].IP.String())
	assert.Equal(t, "00:aa:bb:cc:dd:ee", reserved[0].HwAddr.String())
	assert.Nil(t, reserved[0].Expires)

	other := lease.Client{HwAddr: net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xff}}
	assert.ErrorIs(t, db.Reserve(ctx, net.IP{127, 0, 0, 50}, other), lease.ErrAddressReserved)

	// the plugin needs the database of the server
	srv := test.NewServer(t)
	srv.Database = nil

	p := &Plugin{Addresses: map[string]net.IP{"00:aa:bb:cc:dd:ee": {127, 0, 0, 50}}}
	assert.Error(t, p.Provision(dhcpserver.WithServer(test.NewContext(t), srv)))
}
