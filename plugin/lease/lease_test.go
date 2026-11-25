package lease

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeLeasePlugin sets up the plugin from input and provisions it, like a
// server does when it loads the plugin from its JSON config.
func makeLeasePlugin(t *testing.T, input string) (*leaseTimePlugin, error) {
	p := new(leaseTimePlugin)
	if err := p.UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)); err != nil {
		return nil, err
	}

	m, err := test.Provision(t, p)
	if err != nil {
		return nil, err
	}

	return m.(*leaseTimePlugin), nil
}

func TestLeaseSetup(t *testing.T) {
	durations := map[string]time.Duration{
		"lease 1h":                 time.Hour,
		"lease 3m30s":              3*time.Minute + 30*time.Second,
		"lease 1d":                 24 * time.Hour,
		"lease 1d3m":               24*time.Hour + 3*time.Minute,
		"lease 7d":                 7 * 24 * time.Hour,
		"lease 1.5d":               36 * time.Hour,
		"lease 1h\nlease 2h":       2 * time.Hour,
		"lease 1d12h\nlease 1d13h": 37 * time.Hour,
	}

	for input, expected := range durations {
		p, err := makeLeasePlugin(t, input)
		require.NoError(t, err, input)
		assert.Equal(t, expected, time.Duration(p.LeaseTime), input)
	}

	for _, input := range []string{
		"lease",
		"lease 1y",
		"lease forever",
		"lease 1",
	} {
		_, err := makeLeasePlugin(t, input)
		assert.Error(t, err, input)
	}
}

func TestLeaseServeDHCP(t *testing.T) {
	p, err := makeLeasePlugin(t, "lease 1d")
	require.NoError(t, err)
	p.Wrap(test.NoOpHandler)

	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
	require.NoError(t, err)

	res, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)
	require.NoError(t, p.ServeDHCP(context.Background(), req, res))
	assert.Equal(t, 24*time.Hour, res.IPAddressLeaseTime(0))

	// a lease time that is already set is kept
	res, err = dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)
	res.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Hour))
	require.NoError(t, p.ServeDHCP(context.Background(), req, res))
	assert.Equal(t, time.Hour, res.IPAddressLeaseTime(0))
}
