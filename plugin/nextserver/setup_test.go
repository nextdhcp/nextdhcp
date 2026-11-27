package nextserver

import (
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextServerSetup(t *testing.T) {
	p := new(nextServerPlugin)
	require.NoError(t, p.UnmarshalCaddyfile(caddyfile.NewTestDispenser("next-server 10.1.1.1")))
	assert.Equal(t, "10.1.1.1", p.NextServer.String())
	assert.NoError(t, p.Validate())

	for _, input := range []string{
		"next-server",
		"next-server tftp.example.com",
		"next-server 10.1.1.1 10.1.1.2",
		// the server address field of DHCPv4 messages only holds IPv4
		// addresses
		"next-server fe80::1",
	} {
		assert.Error(t, new(nextServerPlugin).UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)), input)
	}
}

// TestNextServerValidate checks that a JSON config with a next server that
// isn't an IPv4 address fails to load, as UnmarshalCaddyfile rejects it in a
// Dhcpfile.
func TestNextServerValidate(t *testing.T) {
	assert.NoError(t, (&nextServerPlugin{}).Validate())
	assert.NoError(t, (&nextServerPlugin{NextServer: net.ParseIP("10.1.1.1")}).Validate())
	assert.Error(t, (&nextServerPlugin{NextServer: net.ParseIP("fe80::1")}).Validate())
}
