package static

import (
	"net"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
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
}
