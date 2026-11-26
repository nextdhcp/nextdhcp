package test

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/core/lease/storage/drivers/memory"
	"github.com/stretchr/testify/require"
)

// TestSubnet is the subnet that the servers of CreateTestBed and Provision
// serve.
const TestSubnet = "127.0.0.1/8"

// CreateTestBed creates a dhcpserver.Helper for testing the setup function of
// a directive. Its dispenser reads input, and its server is a server for
// TestSubnet, as a server block for that subnet would be adapted to.
func CreateTestBed(t *testing.T, input string) dhcpserver.Helper {
	t.Helper()

	return dhcpserver.Helper{
		Dispenser: caddyfile.NewTestDispenser(input),
		Server:    &dhcpserver.Server{Subnet: TestSubnet},
		Config:    &caddy.Config{},
	}
}

// NewContext returns a context for provisioning modules in tests. It is
// canceled, which cleans up the modules provisioned with it, when the test
// ends.
func NewContext(t *testing.T) caddy.Context {
	t.Helper()

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	t.Cleanup(cancel)

	return ctx
}

// NewServer returns a server for TestSubnet, like the servers Provision
// provisions modules for. Its lease database keeps leases in memory.
func NewServer(t *testing.T) *dhcpserver.Server {
	t.Helper()

	ip, ipNet, err := net.ParseCIDR(TestSubnet)
	require.NoError(t, err)

	return &dhcpserver.Server{
		Subnet:   TestSubnet,
		IP:       ip,
		Network:  *ipNet,
		Database: storage.NewDatabase(memory.New()),
	}
}

// Provision provisions the handler module m the way a server for TestSubnet
// does when it loads m from its JSON config: it encodes m to JSON, decodes
// the JSON into a new instance of the module and provisions the new
// instance, which it returns. The server is one NewServer returns.
func Provision(t *testing.T, m caddy.Module) (caddy.Module, error) {
	t.Helper()

	raw, err := json.Marshal(m)
	require.NoError(t, err)

	ctx := dhcpserver.WithServer(NewContext(t), NewServer(t))

	mod, err := ctx.LoadModuleByID(string(m.CaddyModule().ID), raw)
	if err != nil {
		return nil, err
	}

	return mod.(caddy.Module), nil
}

// ProvisionServer provisions srv the way the dhcp app does when it loads srv
// from its JSON config: it encodes srv to JSON, decodes the JSON into a new
// server and provisions the new server, which it returns. The server is
// cleaned up when the test ends.
func ProvisionServer(t *testing.T, srv *dhcpserver.Server) (*dhcpserver.Server, error) {
	t.Helper()

	raw, err := json.Marshal(srv)
	require.NoError(t, err)

	provisioned := new(dhcpserver.Server)
	require.NoError(t, json.Unmarshal(raw, provisioned))

	t.Cleanup(func() {
		require.NoError(t, provisioned.Cleanup())
	})

	return provisioned, provisioned.Provision(NewContext(t))
}
