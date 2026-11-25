package dhcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// all directives and lease database drivers
	_ "github.com/nextdhcp/nextdhcp/core"
)

// allDirectives is a Dhcpfile that uses every directive. The directives are
// in a different order than they are set up in.
const allDirectives = `
127.0.0.1/8 {
	range 127.0.0.100 127.0.0.200
	static 00:aa:bb:cc:dd:ee 127.0.0.50
	lease 1h
	bootfile {
		bios pxelinux.0
		uefi bootx64.efi
	}
	next-server 127.0.0.2
	servername nextdhcp
	option router 127.0.0.1
	option {
		nameserver 127.0.0.1
	}
	mqtt {
		name local
		broker tcp://127.0.0.1:1883
		topic nextdhcp/{msgtype}
		payload "{hwaddr}"
	}
	mqtt msgtype == 'REQUEST' {
		use local
		topic nextdhcp/requests
		payload-from echo {hwaddr}
	}
	gotify msgtype == 'DISCOVER' {
		message "{hwaddr} asks for an address"
		title "new client"
		server http://127.0.0.1:1 some-token
	}
	interface lo
	database memory
	log debug
}
`

func adapt(t *testing.T, dhcpfile string) (*caddy.Config, *dhcpserver.App, error) {
	t.Helper()

	cfgJSON, warnings, err := dhcpserver.Adapter{}.Adapt([]byte(dhcpfile), map[string]any{"filename": "Dhcpfile"})
	if err != nil {
		return nil, nil, err
	}
	assert.Empty(t, warnings)

	cfg := new(caddy.Config)
	require.NoError(t, json.Unmarshal(cfgJSON, cfg))

	app := new(dhcpserver.App)
	require.NoError(t, json.Unmarshal(cfg.AppsRaw["dhcp"], app))

	return cfg, app, nil
}

func handlerNames(t *testing.T, srv *dhcpserver.Server) []string {
	t.Helper()

	var names []string
	for _, raw := range srv.HandlersRaw {
		var h struct {
			Handler string `json:"handler"`
		}
		require.NoError(t, json.Unmarshal(raw, &h))
		names = append(names, h.Handler)
	}

	return names
}

func TestAdaptAllDirectives(t *testing.T) {
	cfg, app, err := adapt(t, allDirectives)
	require.NoError(t, err)

	// NextDHCP doesn't run Caddy's admin endpoint or save the config
	require.NotNil(t, cfg.Admin)
	assert.True(t, cfg.Admin.Disabled)
	require.NotNil(t, cfg.Admin.Config)
	require.NotNil(t, cfg.Admin.Config.Persist)
	assert.False(t, *cfg.Admin.Config.Persist)

	// log sets the level of the default log, which doesn't include the
	// logs of the admin endpoint
	require.NotNil(t, cfg.Logging)
	require.Contains(t, cfg.Logging.Logs, caddy.DefaultLoggerName)
	assert.Equal(t, "DEBUG", cfg.Logging.Logs[caddy.DefaultLoggerName].Level)
	assert.Equal(t, []string{"admin"}, cfg.Logging.Logs[caddy.DefaultLoggerName].Exclude)

	require.Len(t, app.Servers, 1)
	srv := app.Servers[0]
	assert.Equal(t, "127.0.0.1/8", srv.Subnet)
	assert.Equal(t, "lo", srv.Interface)
	require.NotNil(t, srv.DatabaseConfig)
	assert.Equal(t, "memory", srv.DatabaseConfig.Driver)

	// handlers are chained in the order of dhcpserver.Directives
	assert.Equal(t, []string{
		"gotify", "mqtt", "option", "servername", "next_server",
		"bootfile", "lease", "static", "range",
	}, handlerNames(t, srv))

	// the adapted config provisions; this decodes every handler from its JSON
	assert.NoError(t, caddy.Validate(cfg))
}

func TestAdaptRangeKey(t *testing.T) {
	_, app, err := adapt(t, `
127.0.0.10 - 127.0.0.20
range 127.0.0.30 127.0.0.40
database memory
`)
	require.NoError(t, err)
	require.Len(t, app.Servers, 1)

	srv := app.Servers[0]

	// the subnet is the one of the interface that has the range
	assert.Equal(t, "127.0.0.1/8", srv.Subnet)

	require.Len(t, srv.HandlersRaw, 1)

	var rng struct {
		Handler string `json:"handler"`
		Ranges  []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"ranges"`
	}
	require.NoError(t, json.Unmarshal(srv.HandlersRaw[0], &rng))
	assert.Equal(t, "range", rng.Handler)
	require.Len(t, rng.Ranges, 2)
	assert.Equal(t, "127.0.0.10", rng.Ranges[0].Start)
	assert.Equal(t, "127.0.0.20", rng.Ranges[0].End)
	assert.Equal(t, "127.0.0.30", rng.Ranges[1].Start)
	assert.Equal(t, "127.0.0.40", rng.Ranges[1].End)
}

func TestAdaptMultipleServerBlocks(t *testing.T) {
	_, app, err := adapt(t, `
127.0.0.1/8 {
	database memory
	lease 1h
}

lo {
	database memory
	servername nextdhcp
}
`)
	require.NoError(t, err)
	require.Len(t, app.Servers, 2)

	assert.Equal(t, "127.0.0.1/8", app.Servers[0].Subnet)
	assert.Equal(t, []string{"lease"}, handlerNames(t, app.Servers[0]))

	assert.Equal(t, "lo", app.Servers[1].Subnet)
	assert.Equal(t, []string{"servername"}, handlerNames(t, app.Servers[1]))
}

func TestAdaptErrors(t *testing.T) {
	cases := map[string]string{
		"unknown directive":       "127.0.0.1/8 {\n\tunknown-directive foo\n}",
		"directive error":         "127.0.0.1/8 {\n\tlease forever\n}",
		"no server block key":     "{\n\tlease 1h\n}",
		"too many keys":           "127.0.0.1/8 127.0.0.2/8 {\n\tlease 1h\n}",
		"unknown subnet":          "interfaceThatDoesNotExist {\n\tlease 1h\n}",
		"range end not in subnet": "127.0.0.10 - 10.0.0.1 {\n\tlease 1h\n}",
		"unknown database driver": "127.0.0.1/8 {\n\tdatabase invalid-driver\n}",
		"syntax error":            "127.0.0.1/8 {\n\tlease 1h\n",
	}

	for name, dhcpfile := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := adapt(t, dhcpfile)
			assert.Error(t, err)
		})
	}
}
