package ranges

import (
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/lease/iprange"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
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
