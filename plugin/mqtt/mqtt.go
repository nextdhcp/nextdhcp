package mqtt

import (
	"context"
	"fmt"
	"strings"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/plugin"
	"github.com/nextdhcp/nextdhcp/plugin/logger"
)

type (
	// msgFactory creates the gotify notification message
	// from the given request and response DHCPv4 messages
	msgFactory func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error)

	mqttConnConfig struct {
		broker       []string
		user         string
		password     string
		clientID     string
		cleanSession bool
		qos          int

		l sync.Mutex
		c mqtt.Client
	}

	mqttConfig struct {
		*matcher.Matcher

		conn    *mqttConnConfig
		name    string // optional name for the mqtt config
		topic   msgFactory
		payload msgFactory
	}

	mqttPlugin struct {
		configs []*mqttConfig
		next    plugin.Handler
	}
)

// Name returns "mqtt" and implements plugin.Handler
func (m *mqttPlugin) Name() string {
	return "mqtt"
}

// ServeDHCP forwards the DHCP request and sends any MQTT notifications configured.
// It implements plugin.Handler
func (m *mqttPlugin) ServeDHCP(ctx context.Context, req *dhcpv4.DHCPv4, resp *dhcpv4.DHCPv4) error {
	log.With(ctx)
	if err := m.next.ServeDHCP(ctx, req, resp); err != nil {
		return err
	}

	for _, cfg := range m.configs {
		go func(cfg *mqttConfig) {
			match, err := cfg.Match(ctx, req)
			if err != nil {
				logger.Log.Errorf("matching failed for MQTT plugin with name %q: %s", cfg.name, err.Error())
				return
			}

			if match {
				cli, qos, err := m.getClient(cfg)
				if err != nil {
					logger.Log.Errorf("failed to get MQTT connection for %q: %s", cfg.name, err.Error())
					return
				}

				topic, err := cfg.topic(ctx, req, nil)
				if err != nil {
					logger.Log.Errorf("failed to get MQTT topic for %q: %s", cfg.name, err.Error())
					return
				}

				payload, err := cfg.payload(ctx, req, nil)
				if err != nil {
					logger.Log.Errorf("failed to get MQTT topic for %q: %s", cfg.name, err.Error())
					return
				}

				if token := cli.Publish(topic, byte(qos), false, payload); token.Wait() && token.Error() != nil {
					logger.Log.Errorf("failed to publish MQTT message for %q: %s", cfg.name, token.Error())
					return
				}

				logger.Log.Debugf("published MQTT message to topic %s", topic)
			}
		}(cfg)
	}

	return nil
}

func (m *mqttPlugin) getClient(cfg *mqttConfig) (mqtt.Client, int, error) {
	// check if we should use a different configuration
	if cfg.name != "" && cfg.conn == nil {
		for _, c := range m.configs {
			if c.name == cfg.name && c.conn != nil {
				return m.getClient(c)
			}
		}
		return nil, 0, fmt.Errorf("MQTT configuration with name %q not found", cfg.name)
	}

	cfg.conn.l.Lock()
	defer cfg.conn.l.Unlock()

	if cfg.conn.c == nil {
		if err := cfg.conn.open(); err != nil {
			return nil, 0, err
		}
	}

	return cfg.conn.c, cfg.conn.qos, nil
}

func (conn *mqttConnConfig) open() error {
	opts := mqtt.NewClientOptions()

	for _, b := range conn.broker {
		opts.AddBroker(b)
	}

	if conn.user != "" {
		opts.SetUsername(conn.user)
	}

	if conn.password != "" {
		opts.SetPassword(conn.password)
	}

	if conn.cleanSession {
		opts.SetCleanSession(true)
	}

	if conn.clientID != "" {
		opts.SetClientID(conn.clientID)
	}

	opts.SetAutoReconnect(true)

	cli := mqtt.NewClient(opts)

	var servers []string
	for _, s := range opts.Servers {
		servers = append(servers, s.String())
	}

	logger.Log.Debugf("connecting to MQTT brokers at %s", strings.Join(servers, ", "))
	if token := cli.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	logger.Log.Infof("connected to MQTT brokers at %s", strings.Join(servers, ", "))

	conn.c = cli

	return nil
}
