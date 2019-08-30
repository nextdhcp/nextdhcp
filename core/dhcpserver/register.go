package dhcpserver

import (
	"fmt"
	"log"
	"net"

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

func keyForConfig(serverBlockIndex, serverBlockKeyIndex int) string {
	return fmt.Sprintf("%d:%d", serverBlockIndex, serverBlockKeyIndex)
}

// GetConfig gets the Config that corresponds to c
// if none exist nil is returned
func GetConfig(c *caddy.Controller) *Config {
	ctx := c.Context().(*dhcpContext)
	key := keyForConfig(c.ServerBlockIndex, c.ServerBlockKeyIndex)

	cfg := ctx.keyToConfig[key]
	return cfg
}

func (c *dhcpContext) addConfig(key string, cfg *Config) {
	c.configs = append(c.configs, cfg)
	c.keyToConfig[key] = cfg
}

func (c *dhcpContext) InspectServerBlocks(sourceFile string, serverBlocks []caddyfile.ServerBlock) ([]caddyfile.ServerBlock, error) {
	for si, s := range serverBlocks {
		for ki, k := range s.Keys {
			log.Printf("si=%d ki=%d k=%s", si, ki, k)
			ip, ipNet, err := net.ParseCIDR(k)
			if err != nil {
				return nil, fmt.Errorf("Invalid IP network address '%s' in server block %d", k, si)
			}

			cfg := &Config{
				IP:      ip,
				Network: *ipNet,
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
	}
	
	return nil, fmt.Errorf("not yet supported")
}

func findInterface(cfg *Config) bool {
	if cfg.Interface.Name != "" && len(cfg.Interface.HardwareAddr) > 0 {
		return true
	}
	
	iface, err := findInterfaceByIP(cfg.Network.IP)
	if err != nil {
		return false
	}
	
	cfg.Interface = *iface
	return true
}

func findInterfaceByIP(ip net.IP) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.Equal(ip) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to find interface for %s", ip.String())
}