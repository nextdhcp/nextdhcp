package static

import (
	"net"
	"testing"

	_ "github.com/nextdhcp/nextdhcp/core/dhcpserver"

	"github.com/caddyserver/caddy"
	"github.com/stretchr/testify/assert"
)

func TestStaticPluginSetup(t *testing.T) {
	c := caddy.NewTestController("dhcpv4", "static 00:aa:bb:cc:dd:ee 10.0.0.1")
	static, err := makeStaticPlugin(c)
	assert.NoError(t, err)
	assert.Len(t, static.Addresses, 1)
	assert.True(t, static.Addresses["00:aa:bb:cc:dd:ee"].Equal(net.IP{10, 0, 0, 1}))

	c = caddy.NewTestController("dhcpv4", "static 00:aa:bb:cc:dd:ee")
	_, err = makeStaticPlugin(c)
	assert.Error(t, err)

	c = caddy.NewTestController("dhcpv4", "static 00:aa:bb:cc 10.0.0.1")
	_, err = makeStaticPlugin(c)
	assert.Error(t, err)

	c = caddy.NewTestController("dhcpv4", "static 00:aa:bb:cc:dd:ee 10.0")
	_, err = makeStaticPlugin(c)
	assert.Error(t, err)

	cfg := `
	static 00:aa:bb:cc:dd:ee 10.0.0.1
	static 00:bb:cc:dd:ee:ff 10.0.0.1
	`
	c = caddy.NewTestController("dhcpv4", cfg)
	_, err = makeStaticPlugin(c)
	assert.Error(t, err)

	cfg = `
	static 00:aa:bb:cc:dd:ee 10.0.0.1
	static 00:aa:bb:cc:dd:ee 10.0.0.2
	`
	c = caddy.NewTestController("dhcpv4", cfg)
	_, err = makeStaticPlugin(c)
	assert.Error(t, err)
}
