package dhcpserver

import (
	"fmt"
	"net"

	"github.com/apex/log"
	"github.com/caddyserver/caddy"
	"github.com/caddyserver/caddy/caddyfile"
)

const serverType = "dhcpv4"

func init() {
	caddy.RegisterServerType(serverType, caddy.ServerType{
		Directives: func() []string { return Directives },
		DefaultInput: func() caddy.Input {
			return caddy.CaddyfileInput{
				Filepath:       "Dhcpfile",
				Contents:       []byte{},
				ServerTypeName: serverType,
			}
		},
		NewContext: newContext,
	})
}

func newContext(i *caddy.Instance) caddy.Context {
	return &dhcpContext{
		keyToConfig: make(map[string]*Config),
	}
}

type dhcpContext struct {
	configs     []*Config
	keyToConfig map[string]*Config
}

func (c *dhcpContext) addConfig(key string, cfg *Config) {
	c.configs = append(c.configs, cfg)
	c.keyToConfig[key] = cfg
}

func (c *dhcpContext) InspectServerBlocks(sourceFile string, serverBlocks []caddyfile.ServerBlock) ([]caddyfile.ServerBlock, error) {
	for si, s := range serverBlocks {

		if len(s.Keys) == 1 {
			k := s.Keys[0]
			cfg := &Config{
				logger: log.Log,
			}

			if err := tryInterfaceNameOrIP(k, cfg); err != nil {
				return nil, fmt.Errorf("failed to get subnet configuration for server block %s (index = %d)", k, si)
			}

			configKey := keyForConfig(si)
			c.addConfig(configKey, cfg)

			continue
		}

		if len(s.Keys) == 3 && s.Keys[1] == "-" {
			// 10.1.0.1 - 10.1.0.100
			startIP := net.ParseIP(s.Keys[0])
			endIP := net.ParseIP(s.Keys[2])

			iface, ipNet, err := findInterfaceContainingIP(startIP)
			if err != nil {
				return nil, err
			}

			// make sure iface also contains the endIP
			if !ipNet.Contains(endIP) {
				return nil, fmt.Errorf("end of range not included in %s on %s", ipNet.String(), iface.Name)
			}

			cfg := &Config{
				logger:  log.Log,
				IP:      ipNet.IP,
				Network: *ipNet,
			}

			// make sure we add the range plugin now
			// and in front of any other range plugin configuration
			s.Tokens["range"] = append([]caddyfile.Token{
				caddyfile.Token{Text: "range"},
				caddyfile.Token{Text: startIP.String()},
				caddyfile.Token{Text: endIP.String()},
			}, s.Tokens["range"]...)

			configKey := keyForConfig(si)
			c.addConfig(configKey, cfg)

			continue
		}

		return nil, fmt.Errorf("unexpected number of server block keys: %d (keys=%+v)", len(s.Keys), s.Keys)
	}

	return serverBlocks, nil
}

func (c *dhcpContext) MakeServers() ([]caddy.Server, error) {
	for _, c := range c.configs {
		if !findInterface(c) {
			return nil, fmt.Errorf("failed to find interface for subnet %s", c.Network.String())
		}

		if err := ensureDatabase(c); err != nil {
			return nil, fmt.Errorf("failed to open database for subnet %s: %s", c.Network.String(), err.Error())
		}

		if err := buildMiddlewareChain(c); err != nil {
			return nil, fmt.Errorf("failed to build middleware chain for subnet %s: %s", c.Network.String(), err.Error())
		}
	}

	var servers []caddy.Server
	for _, cfg := range c.configs {
		s, err := NewServer(cfg)
		if err != nil {
			return servers, err
		}

		servers = append(servers, s)
	}

	return servers, nil
}
