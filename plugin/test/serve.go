package test

import (
	"testing"

	"github.com/caddyserver/caddy"
	"github.com/caddyserver/caddy/caddyfile"
	"github.com/stretchr/testify/require"
)

// CreateTestBed creates a new caddy.Controller that is configured for
// testing the setup and configuration of plugins. It creates a dummy server
// block in the context of "dhcpv4" server type so plugins can safely assume
// dhcpserver.GetConfig(ctrl) will return a valid configuration. The server
// block itself is configured to serve on 127.0.0.1/8
func CreateTestBed(t *testing.T, input string) *caddy.Controller {
	ctrl := caddy.NewTestController("dhcpv4", input)
	ctx := ctrl.Context()

	serverBlock := caddyfile.ServerBlock{
		Keys:   []string{"127.0.0.1/8"},
		Tokens: map[string][]caddyfile.Token{},
	}

	blks, err := ctx.InspectServerBlocks("test-source", []caddyfile.ServerBlock{serverBlock})
	require.NoError(t, err)
	require.Equal(t, []caddyfile.ServerBlock{serverBlock}, blks)

	return ctrl
}
