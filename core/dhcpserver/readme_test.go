package dhcpserver_test

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadmeExamples checks that the examples in the README of NextDHCP and
// in the READMEs of its plugins work.
func TestReadmeExamples(t *testing.T) {
	readmes, err := filepath.Glob("../../plugin/*/README.md")
	require.NoError(t, err)
	readmes = append([]string{"../../README.md"}, readmes...)

	for _, readme := range readmes {
		examples := test.Examples(t, readme)
		name := strings.TrimPrefix(readme, "../../")

		if name != "plugin/lua/README.md" {
			assert.NotEmpty(t, examples, "%s has no examples", name)
		}

		for i, example := range examples {
			t.Run(fmt.Sprintf("%s/%d", name, i+1), func(t *testing.T) {
				dhcpfile, err := loopbackDhcpfile(example)
				require.NoError(t, err, example)

				cfg, app, err := adapt(t, dhcpfile)
				require.NoError(t, err, dhcpfile)

				// provision the config, with lease databases that don't need
				// any files
				for _, srv := range app.Servers {
					srv.DatabaseConfig = &dhcpserver.DatabaseConfig{Driver: "memory"}
				}
				cfg.AppsRaw["dhcp"] = caddyconfig.JSON(app, nil)

				assert.NoError(t, caddy.Validate(cfg), dhcpfile)
			})
		}
	}
}

var interfaceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// loopbackDhcpfile turns an example into a Dhcpfile that can be adapted on
// any host. It puts an example that is a directive into a server block, and
// replaces the addresses of server blocks with addresses of the loopback
// interface, and interface names with lo, after checking that they are
// valid.
func loopbackDhcpfile(example string) (string, error) {
	if fields := strings.Fields(example); len(fields) > 0 && slices.Contains(dhcpserver.Directives, fields[0]) {
		example = "127.0.0.1/8 {\n" + example + "}\n"
	}

	serverBlocks, err := caddyfile.Parse("example", []byte(example))
	if err != nil {
		return "", err
	}

	lines := strings.Split(example, "\n")

	// The examples have the keys of a server block, and its opening brace,
	// on one line
	for _, sb := range serverBlocks {
		keys, err := loopbackKeys(sb.GetKeysText())
		if err != nil {
			return "", err
		}

		if sb.HasBraces {
			keys += " {"
		}

		lines[sb.Keys[0].Line-1] = keys
	}

	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "interface" {
			if !interfaceName.MatchString(fields[1]) {
				return "", fmt.Errorf("invalid interface name %s", fields[1])
			}

			lines[i] = "interface lo"
		}
	}

	return strings.Join(lines, "\n"), nil
}

func loopbackKeys(keys []string) (string, error) {
	switch {
	case len(keys) == 1:
		ip, ipNet, err := net.ParseCIDR(keys[0])
		if err != nil {
			if !interfaceName.MatchString(keys[0]) {
				return "", fmt.Errorf("server block key %s is neither an address in CIDR notation nor an interface name", keys[0])
			}

			return "lo", nil
		}

		if ip.Equal(ipNet.IP) {
			return "", fmt.Errorf("server block key %s is the address of the network instead of the address of the server", keys[0])
		}

		return "127.0.0.1/8", nil

	case len(keys) == 3 && keys[1] == "-":
		if net.ParseIP(keys[0]).To4() == nil || net.ParseIP(keys[2]).To4() == nil {
			return "", fmt.Errorf("invalid range %s - %s", keys[0], keys[2])
		}

		return "127.0.0.10 - 127.0.0.20", nil

	default:
		return "", fmt.Errorf("unexpected server block keys %v", keys)
	}
}
