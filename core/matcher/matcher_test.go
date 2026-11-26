package matcher

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
)

func TestSetupMatcher(t *testing.T) {
	testCases := []struct {
		input       string
		ifOp        string
		expectedErr bool
	}{
		{
			"msgtype == DISCOVER",
			"and",
			false,
		},
		{
			"",
			"or",
			false,
		},
		{
			"==",
			"",
			true,
		},
	}

	for i, c := range testCases {
		testCase := fmt.Sprintf("case #%d", i+1)
		input := fmt.Sprintf("{\nif %s\nif_op %s\n}", c.input, c.ifOp)
		disp := caddyfile.NewTestDispenser(input)
		m, err := SetupMatcher(disp)
		if c.expectedErr {
			assert.Error(t, err, testCase)
			assert.Nil(t, m, testCase)
		} else {
			assert.NoError(t, err, testCase)
			assert.NotNil(t, m, testCase)
		}
	}
}

// TestSetupMatcherIfOpError checks that an invalid if_op is reported where it
// is in the Dhcpfile.
func TestSetupMatcherIfOpError(t *testing.T) {
	_, err := SetupMatcher(caddyfile.NewTestDispenser("{\nif msgtype == DISCOVER\nif_op xor\n}"))
	assert.ErrorContains(t, err, "Testfile:3")
	assert.ErrorContains(t, err, "'xor'")
}

func TestMatcherWithFunctions(t *testing.T) {
	input := "testFunc(\"arg1\")"
	disp := caddyfile.NewTestDispenser(input)

	fns := map[string]ExprFunc{
		"testFunc": func(args ...interface{}) (interface{}, error) {
			assert.Equal(t, "arg1", args[0])

			return true, nil
		},
	}

	m, err := SetupMatcherRemainingArgs(disp, fns)
	assert.NoError(t, err)
	assert.NotNil(t, m)
}

func TestMatch(t *testing.T) {
	ctx := context.Background()
	req, _ := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})

	cases := []struct {
		I string
		R bool
		E bool
	}{
		{
			I: "true",
			R: true,
			E: false,
		},
		{
			I: "false",
			R: false,
			E: false,
		},
		{
			I: "1 == 2",
			R: false,
			E: false,
		},
		{
			I: "1 == 1",
			R: true,
			E: false,
		},
		{
			I: "msgtype == 'DISCOVER'",
			R: true,
		},
		{
			I: "msgtype == 'REQUEST'",
			R: false,
		},
	}

	for i, c := range cases {
		disp := caddyfile.NewTestDispenser(c.I)
		m, err := SetupMatcherRemainingArgs(disp)
		assert.NoError(t, err)

		res, err := m.Match(ctx, req)
		if c.E == false {
			assert.Equal(t, c.R, res, "case %d: %s", i, c.I)
			assert.NoError(t, err, "case %d: %s", i, c.I)
		} else {
			assert.Error(t, err, "case %d: %s", i, c.I)
		}
	}
}
