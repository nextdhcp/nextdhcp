package option

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
)

func TestCustomOption(t *testing.T) {
	cases := []struct {
		Name    string
		Value   []string
		Code    uint8
		Payload []byte
		Err     bool
	}{
		{
			"0xaa",
			[]string{"0xaabbccdd", "0xeeff"},
			0xaa,
			[]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			false,
		},
		{
			"0x88",
			[]string{"fe"},
			0x88,
			[]byte{0xfe},
			false,
		},
		{
			"fo",
			nil,
			0,
			nil,
			true,
		},
		{
			"0xaa",
			[]string{"fae"},
			0xaa,
			nil,
			true,
		},
	}

	for idx, c := range cases {
		o, v, err := parseCustomOption(c.Name, c.Value)

		if err == nil {
			assert.Equal(t, c.Code, o.Code(), "case %d: code does not match", idx)
			assert.Equal(t, c.Payload, v.ToBytes(), "case %d: payload does not match", idx)
			assert.False(t, c.Err, "case %d: expected an error", idx)
		} else {
			assert.True(t, c.Err, "case %d: did not expect an error but got %s", idx, err.Error())
		}
	}
}

func getController(input string) *caddyfile.Dispenser {
	return caddyfile.NewTestDispenser(input)
}

func unmarshalOption(c *caddyfile.Dispenser) error {
	return new(Plugin).UnmarshalCaddyfile(c)
}

func TestSetupPlugin(t *testing.T) {
	cases := []struct {
		I string
		O map[dhcpv4.OptionCode]dhcpv4.OptionValue
		E bool
	}{}

	for _, c := range cases {
		ctrl := getController(c.I)

		err := unmarshalOption(ctrl)
		if c.E {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}

	}
}

// TestProvisionOptionWithoutValues checks that a JSON config with an option
// that takes one value, but has none, fails to load instead of panicking.
func TestProvisionOptionWithoutValues(t *testing.T) {
	assert.NotPanics(t, func() {
		_, err := test.Provision(t, &Plugin{OptionsRaw: []OptionConfig{{Name: "hostname"}}})
		assert.Error(t, err)
	})
}
