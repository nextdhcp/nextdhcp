package dhcpserver

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/core/utils/iface"
)

// DefaultConfigFile is the name of the Dhcpfile that is loaded if no other
// one is given.
const DefaultConfigFile = "Dhcpfile"

func init() {
	caddyconfig.RegisterAdapter("dhcpfile", Adapter{})
}

// Adapter adapts a Dhcpfile to a Caddy JSON config with a "dhcp" app. It is
// registered as the "dhcpfile" config adapter.
type Adapter struct{}

// Adapt implements caddyconfig.Adapter. The "filename" option names the
// Dhcpfile in error messages.
func (Adapter) Adapt(body []byte, options map[string]any) ([]byte, []caddyconfig.Warning, error) {
	filename, _ := options["filename"].(string)
	if filename == "" {
		filename = DefaultConfigFile
	}

	serverBlocks, err := caddyfile.Parse(filename, body)
	if err != nil {
		return nil, nil, err
	}

	cfg, warnings, err := ServerType{}.Setup(serverBlocks, options)
	if err != nil {
		return nil, warnings, err
	}

	result, err := json.Marshal(cfg)

	return result, warnings, err
}

// ServerType sets up a Caddy config from the server blocks of a Dhcpfile.
// Each server block becomes one server of the "dhcp" app.
type ServerType struct{}

// Setup implements caddyfile.ServerType.
func (ServerType) Setup(serverBlocks []caddyfile.ServerBlock, _ map[string]any) (*caddy.Config, []caddyconfig.Warning, error) {
	var warnings []caddyconfig.Warning

	persist := false
	cfg := &caddy.Config{
		// NextDHCP serves DHCP only: it doesn't open Caddy's admin endpoint
		// and doesn't save the config to disk.
		Admin: &caddy.AdminConfig{
			Disabled: true,
			Config:   &caddy.ConfigSettings{Persist: &persist},
		},
		Logging: &caddy.Logging{
			Logs: map[string]*caddy.CustomLog{
				// don't warn that the admin endpoint is disabled
				caddy.DefaultLoggerName: {Exclude: []string{"admin"}},
			},
		},
	}

	app := new(App)
	for i, sb := range serverBlocks {
		srv, err := setupServerBlock(i, sb, cfg, &warnings)
		if err != nil {
			return nil, warnings, err
		}

		app.Servers = append(app.Servers, srv)
	}

	cfg.AppsRaw = caddy.ModuleMap{
		"dhcp": caddyconfig.JSON(app, &warnings),
	}

	return cfg, warnings, nil
}

func setupServerBlock(index int, sb caddyfile.ServerBlock, cfg *caddy.Config, warnings *[]caddyconfig.Warning) (*Server, error) {
	keys := sb.GetKeysText()
	srv := new(Server)

	switch {
	case len(keys) == 1:
		if _, _, err := iface.ByNameOrCIDR(keys[0]); err != nil {
			return nil, fmt.Errorf("failed to get subnet configuration for server block %s (index = %d): %s", keys[0], index, err.Error())
		}

		srv.Subnet = keys[0]

	case len(keys) == 3 && keys[1] == "-":
		// 10.1.0.1 - 10.1.0.100
		startIP := net.ParseIP(keys[0])
		endIP := net.ParseIP(keys[2])

		ifc, ipNet, err := iface.Contains(startIP)
		if err != nil {
			return nil, err
		}

		// make sure iface also contains the endIP
		if !ipNet.Contains(endIP) {
			return nil, fmt.Errorf("end of range not included in %s on %s", ipNet.String(), ifc.Name)
		}

		srv.Subnet = ipNet.String()

		// make sure we add the range plugin now
		// and in front of any other range plugin configuration
		key := sb.Keys[0]
		sb.Segments = append([]caddyfile.Segment{{
			{File: key.File, Line: key.Line, Text: "range"},
			{File: key.File, Line: key.Line, Text: startIP.String()},
			{File: key.File, Line: key.Line, Text: endIP.String()},
		}}, sb.Segments...)

	default:
		return nil, fmt.Errorf("unexpected number of server block keys: %d (keys=%+v)", len(keys), keys)
	}

	for _, seg := range sb.Segments {
		if _, ok := directives[seg.Directive()]; !ok {
			return nil, fmt.Errorf("%s:%d - Error during parsing: Unknown directive '%s'", seg[0].File, seg[0].Line, seg.Directive())
		}
	}

	// Directives are set up in the order of Directives, which is the order of
	// the handler chain, not in the order of the Dhcpfile.
	for _, name := range Directives {
		setup, ok := directives[name]
		if !ok || !slices.ContainsFunc(sb.Segments, func(seg caddyfile.Segment) bool { return seg.Directive() == name }) {
			continue
		}

		h := Helper{
			Dispenser: sb.DispenseDirective(name),
			Server:    srv,
			Config:    cfg,
			warnings:  warnings,
		}

		if err := setup(h); err != nil {
			return nil, err
		}
	}

	return srv, nil
}

// Helper is passed to the setup function of a directive. Its dispenser reads
// all occurrences of the directive in one server block, in the order they
// appear, so setup functions typically loop over them with `for h.Next()`.
type Helper struct {
	*caddyfile.Dispenser

	// Server is the server the server block is adapted to. Directives that
	// configure the server itself, instead of adding a handler, modify it.
	Server *Server

	// Config is the Caddy config the Dhcpfile is adapted to.
	Config *caddy.Config

	warnings *[]caddyconfig.Warning
}

// AddHandler appends the handler module m to the server's handler chain. m
// must implement plugin.Middleware.
func (h Helper) AddHandler(m caddy.Module) error {
	id := m.CaddyModule().ID

	raw := caddyconfig.JSONModuleObject(m, "handler", id.Name(), h.warnings)
	if raw == nil {
		return fmt.Errorf("failed to encode the configuration of %s", id)
	}

	h.Server.HandlersRaw = append(h.Server.HandlersRaw, raw)

	return nil
}

type (
	// DirectiveFunc sets up a directive from all its occurrences in one
	// server block.
	DirectiveFunc func(h Helper) error

	// HandlerDirectiveFunc sets up a directive that adds a handler to the
	// server's chain, from all its occurrences in one server block. It returns
	// the handler module, which must implement plugin.Middleware.
	HandlerDirectiveFunc func(h Helper) (caddy.Module, error)
)

var directives = map[string]DirectiveFunc{}

// RegisterDirective registers the setup function of a directive. The
// directive must be listed in Directives. It panics if the directive is not
// listed or is already registered.
func RegisterDirective(name string, setup DirectiveFunc) {
	if !slices.Contains(Directives, name) {
		panic(fmt.Sprintf("directive %s is not listed in dhcpserver.Directives", name))
	}

	if _, ok := directives[name]; ok {
		panic(fmt.Sprintf("directive %s is already registered", name))
	}

	directives[name] = setup
}

// RegisterHandlerDirective registers the setup function of a directive that
// adds a handler to the server's chain. See RegisterDirective.
func RegisterHandlerDirective(name string, setup HandlerDirectiveFunc) {
	RegisterDirective(name, func(h Helper) error {
		m, err := setup(h)
		if err != nil {
			return err
		}

		return h.AddHandler(m)
	})
}

// Interface guards
var (
	_ caddyconfig.Adapter  = Adapter{}
	_ caddyfile.ServerType = ServerType{}
)
