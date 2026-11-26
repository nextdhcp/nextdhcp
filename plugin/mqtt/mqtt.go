package mqtt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type (
	// msgFactory creates the gotify notification message
	// from the given request and response DHCPv4 messages
	msgFactory func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error)

	mqttConnection struct {
		broker       []string
		user         string
		password     string
		clientID     string
		cleanSession bool
		qos          int

		l          sync.Mutex    // protects the fields below
		c          mqtt.Client   // the connected client, if any
		connecting chan struct{} // closed when the connection attempt in progress ends
		closed     bool          // set by Cleanup, after which no connection is opened
	}

	mqttConfig struct {
		*matcher.Matcher

		conn    *mqttConnection
		name    string // optional name for the mqtt config
		topic   msgFactory
		payload msgFactory
	}

	mqttPlugin struct {
		// Publish configures the messages to publish, in order.
		Publish []PublishConfig `json:"publish,omitempty"`

		configs []*mqttConfig
		l       log.Logger
		next    plugin.Handler
		wg      sync.WaitGroup // used in tests to wait until all messages are published
	}
)

// Name returns "mqtt" and implements plugin.Handler
func (m *mqttPlugin) Name() string {
	return "mqtt"
}

// ServeDHCP forwards the DHCP request and sends any MQTT notifications configured.
// It implements plugin.Handler
func (m *mqttPlugin) ServeDHCP(ctx context.Context, req *dhcpv4.DHCPv4, resp *dhcpv4.DHCPv4) error {
	l := log.With(ctx, m.l)
	if err := m.next.ServeDHCP(ctx, req, resp); err != nil {
		return err
	}

	for _, cfg := range m.configs {
		// a configuration without topic only configures a connection for
		// other configurations to use
		if cfg.topic == nil {
			continue
		}

		m.wg.Add(1)
		go func(cfg *mqttConfig) {
			defer m.wg.Done()

			match, err := cfg.Match(ctx, req)
			if err != nil {
				l.Errorf("matching failed for MQTT plugin with name %q: %s", cfg.name, err.Error())
				return
			}

			if match {
				cli, qos, err := m.getClient(cfg)
				if err != nil {
					l.Errorf("failed to get MQTT connection for %q: %s", cfg.name, err.Error())
					return
				}

				topic, err := cfg.topic(ctx, req, nil)
				if err != nil {
					l.Errorf("failed to get MQTT topic for %q: %s", cfg.name, err.Error())
					return
				}

				payload, err := cfg.payload(ctx, req, nil)
				if err != nil {
					l.Errorf("failed to get MQTT payload for %q: %s", cfg.name, err.Error())
					return
				}

				if token := cli.Publish(topic, byte(qos), false, payload); token.Wait() && token.Error() != nil {
					l.Errorf("failed to publish MQTT message for %q: %s", cfg.name, token.Error())
					return
				}

				l.Debugf("published MQTT message to topic %s", topic)
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

	cli, err := cfg.conn.client(m.l)
	if err != nil {
		return nil, 0, err
	}

	return cli, cfg.conn.qos, nil
}

var errConnectionClosed = errors.New("MQTT connection already closed")

// client returns the connected client, and connects first if there is none.
// It doesn't hold conn.l while it connects, so close doesn't wait for that.
func (conn *mqttConnection) client(l log.Logger) (mqtt.Client, error) {
	for {
		conn.l.Lock()
		switch {
		case conn.closed:
			conn.l.Unlock()
			return nil, errConnectionClosed

		case conn.c != nil:
			cli := conn.c
			conn.l.Unlock()
			return cli, nil

		case conn.connecting != nil:
			// wait for the connection attempt in progress, then check again
			connecting := conn.connecting
			conn.l.Unlock()
			<-connecting
			continue
		}

		connecting := make(chan struct{})
		conn.connecting = connecting
		conn.l.Unlock()

		cli, err := conn.open(l)

		conn.l.Lock()
		conn.connecting = nil
		close(connecting)
		closed := conn.closed
		if err == nil && !closed {
			conn.c = cli
		}
		conn.l.Unlock()

		switch {
		case err != nil:
			return nil, err
		case closed:
			// close ran while connecting, so nothing else would disconnect
			cli.Disconnect(250)
			return nil, errConnectionClosed
		}

		return cli, nil
	}
}

// close disconnects the client and keeps new connections from being opened,
// so messages still being published don't open one that nothing would close.
func (conn *mqttConnection) close() {
	conn.l.Lock()
	conn.closed = true
	cli := conn.c
	conn.c = nil
	conn.l.Unlock()

	if cli != nil {
		cli.Disconnect(250)
	}
}

func (conn *mqttConnection) open(l log.Logger) (mqtt.Client, error) {
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

	l.Debugf("connecting to MQTT brokers at %s", strings.Join(servers, ", "))
	if token := cli.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}
	l.Infof("connected to MQTT brokers at %s", strings.Join(servers, ", "))

	return cli, nil
}
