package dhcpserver

import (
	"fmt"
	"net"

	"github.com/caddyserver/caddy"
	"github.com/caddyserver/caddy/caddyfile"
	"github.com/sirupsen/logrus"
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
		for ki, k := range s.Keys {
			ip, ipNet, err := net.ParseCIDR(k)
			if err != nil {
				return nil, fmt.Errorf("Invalid IP network address '%s' in server block %d", k, si)
			}

			cfg := &Config{
				IP:      ip,
				Network: *ipNet,
				logger:  logrus.New(),
			}

			configKey := keyForConfig(si, ki)
			c.addConfig(configKey, cfg)
		}
	}

	return serverBlocks, nil
}

func (c *dhcpContext) MakeServers() ([]caddy.Server, error) {
	for _, c := range c.configs {
		if !findInterface(c) {
			return nil, fmt.Errorf("failed to find interface for subnet %s", c.Network.String())
		}

		if err := openDatabase(c); err != nil {
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
