package ranges

import (
	"context"
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/iprange"
	"github.com/nextdhcp/nextdhcp/core/lease/mockdb"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestRangeValidation checks that ranges must be of IPv4 addresses and must
// not start after their end, in the Dhcpfile and in the JSON config.
func TestRangeValidation(t *testing.T) {
	for _, input := range []string{
		"range 10.0.0.10 10.0.0.20",
		"range 10.0.0.10 10.0.0.10",
	} {
		p := new(RangePlugin)
		require.NoError(t, p.UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)), input)

		_, err := test.Provision(t, p)
		assert.NoError(t, err, input)
	}

	for _, input := range []string{
		"range fd00::10 fd00::20",
		"range 10.0.0.10 fd00::20",
		"range 10.0.0.20 10.0.0.10",
	} {
		err := new(RangePlugin).UnmarshalCaddyfile(caddyfile.NewTestDispenser(input))
		assert.ErrorContains(t, err, "Testfile:1", input)
	}

	for _, r := range []*iprange.IPRange{
		{Start: net.ParseIP("fd00::10"), End: net.ParseIP("fd00::20")},
		{Start: net.IP{10, 0, 0, 20}, End: net.IP{10, 0, 0, 10}},
		nil,
	} {
		_, err := test.Provision(t, &RangePlugin{Ranges: iprange.IPRanges{r}})
		assert.Error(t, err, "%v", r)
	}
}

// TestFindUnboundAddrWithoutHardwareAddress checks that the plugin stops
// looking for an address for a client without hardware address once the
// database refuses the client, instead of trying each address of its ranges.
func TestFindUnboundAddrWithoutHardwareAddress(t *testing.T) {
	p := &RangePlugin{
		Ranges: iprange.IPRanges{{Start: net.IP{10, 0, 0, 1}, End: net.IP{10, 0, 0, 100}}},
		L:      zerolog.Nop(),
	}

	for _, requested := range []net.IP{nil, {10, 0, 0, 5}} {
		db := new(mockdb.MockDatabase)
		db.On("Reserve", mock.Anything, mock.Anything).Return(lease.ErrNoHardwareAddress)

		assert.Nil(t, p.findUnboundAddr(context.Background(), net.HardwareAddr{}, requested, db))
		db.AssertNumberOfCalls(t, "Reserve", 1)
	}
}
