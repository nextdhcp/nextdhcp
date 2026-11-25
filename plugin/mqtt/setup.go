package mqtt

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/core/replacer"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(new(mqttPlugin))
	dhcpserver.RegisterHandlerDirective("mqtt", setupMqtt)
}

type (
	// PublishConfig configures an MQTT message that is published for
	// matching requests.
	PublishConfig struct {
		// Condition is the condition a request must match for the message to
		// be published. An empty condition matches all requests.
		Condition string `json:"condition,omitempty"`

		// Name is the name of the connection: the name of the connection that
		// Connection configures, or, if Connection is not set, the name of the
		// connection of another message to use.
		Name string `json:"name,omitempty"`

		// Connection configures the connection to the MQTT brokers.
		Connection *ConnectionConfig `json:"connection,omitempty"`

		// Topic is the topic to publish to. Placeholders in it are replaced
		// with values of the request.
		Topic *string `json:"topic,omitempty"`

		// Payload is the payload of the message. Placeholders in it are
		// replaced with values of the request.
		Payload *string `json:"payload,omitempty"`

		// PayloadFrom is a command whose output is used as the payload,
		// instead of Payload. Placeholders in it are replaced with values of
		// the request. It's an unofficial option that may be removed.
		PayloadFrom []string `json:"payload_from,omitempty"`
	}

	// ConnectionConfig configures a connection to MQTT brokers.
	ConnectionConfig struct {
		Broker       []string `json:"broker,omitempty"`
		User         string   `json:"user,omitempty"`
		Password     string   `json:"password,omitempty"`
		CleanSession bool     `json:"clean_session,omitempty"`
		QoS          int      `json:"qos,omitempty"`
	}
)

// validate checks that cfg either publishes messages or only configures a
// named connection, without condition or payload, for others to use.
func (cfg PublishConfig) validate() error {
	if cfg.Connection == nil && cfg.Name == "" {
		return errors.New("Either configure a MQTT connection or \"use\" an existing one")
	}

	onlyConnection := cfg.Connection != nil && cfg.Name != "" &&
		cfg.Condition == "" && cfg.Payload == nil && cfg.PayloadFrom == nil

	if cfg.Topic == nil && !onlyConnection {
		return errors.New("a topic is required, unless the block only configures a named connection for other blocks to use")
	}

	return nil
}

// CaddyModule returns the Caddy module information.
func (*mqttPlugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.mqtt",
		New: func() caddy.Module { return new(mqttPlugin) },
	}
}

// Provision sets up the configured messages.
func (m *mqttPlugin) Provision(ctx caddy.Context) error {
	m.l = ctx.Logger().Sugar()
	m.configs = nil

	for _, pub := range m.Publish {
		if err := pub.validate(); err != nil {
			return err
		}

		cond, err := matcher.SetupMatcherString(pub.Condition)
		if err != nil {
			return err
		}

		cfg := &mqttConfig{
			Matcher: cond,
			name:    pub.Name,
		}

		if pub.Connection != nil {
			cfg.conn = &mqttConnConfig{
				broker:       pub.Connection.Broker,
				user:         pub.Connection.User,
				password:     pub.Connection.Password,
				cleanSession: pub.Connection.CleanSession,
				qos:          pub.Connection.QoS,
			}
		}

		if pub.Topic != nil {
			cfg.topic = getStringFactory(*pub.Topic)

			// the payload is empty unless one is configured
			cfg.payload = getStringFactory("")
		}

		if pub.Payload != nil {
			cfg.payload = getStringFactory(*pub.Payload)
		}

		if pub.PayloadFrom != nil {
			//
			// TODO(ppacher): payload-from allows to execute an external script and use it's output
			// to publish on MQTT. Is this really required? We could also just provide an "exec" plugin
			// that calls an external binary/script and use that for publishing to MQTT.
			//
			m.l.Warnf("payload-from: use of unofficial directive detected")
			m.l.Warnf("payload-from: this directive may vanish in future versions.")

			cfg.payload = getExecCmdStringFactory(pub.PayloadFrom)
		}

		m.configs = append(m.configs, cfg)
	}

	return nil
}

// Cleanup disconnects from the MQTT brokers.
func (m *mqttPlugin) Cleanup() error {
	for _, cfg := range m.configs {
		if cfg.conn == nil {
			continue
		}

		cfg.conn.l.Lock()
		// keep messages still being published from opening a new
		// connection that nothing would close
		cfg.conn.closed = true
		if cfg.conn.c != nil {
			cfg.conn.c.Disconnect(250)
			cfg.conn.c = nil
		}
		cfg.conn.l.Unlock()
	}

	return nil
}

// Wrap implements plugin.Middleware.
func (m *mqttPlugin) Wrap(next plugin.Handler) plugin.Handler {
	m.next = next
	return m
}

func setupMqtt(h dhcpserver.Helper) (caddy.Module, error) {
	plg := new(mqttPlugin)
	return plg, plg.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all mqtt directives of a server
// block.
func (m *mqttPlugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	for c.Next() {
		cfg := PublishConfig{}
		useExisting := false

		cfg.Condition = strings.Join(c.RemainingArgs(), " ")
		if _, err := matcher.SetupMatcherString(cfg.Condition); err != nil {
			return err
		}

		for c.NextBlock(0) {
			switch c.Val() {
			case "name", "broker", "user", "password",
				"clean-session", "qos":
				if useExisting {
					return c.SyntaxErr("either configure a new connection or \"use\" and existing one")
				}

				if err := parseConnectionSettings(&cfg, c); err != nil {
					return err
				}

			case "use":
				if cfg.Connection != nil {
					return c.SyntaxErr("either configure a new connection or \"use\" and existing one")
				}
				useExisting = true

				if !c.NextArg() {
					return c.ArgErr()
				}
				cfg.Name = c.Val()

			case "topic":
				if !c.NextArg() {
					return c.ArgErr()
				}

				topic := c.Val()
				cfg.Topic = &topic

			case "payload", "body":
				if !c.NextArg() {
					return c.ArgErr()
				}

				payload := c.Val()
				cfg.Payload = &payload
				cfg.PayloadFrom = nil

			case "payload-from":
				cmd := c.RemainingArgs()
				if len(cmd) == 0 {
					return c.ArgErr()
				}

				cfg.PayloadFrom = cmd
				cfg.Payload = nil

			default:
				return c.Errf("unknown mqtt property %q", c.Val())
			}
		}

		if !useExisting && cfg.Connection == nil {
			return c.SyntaxErr("Either configure a MQTT connection or \"use\" an existing one")
		}

		if err := cfg.validate(); err != nil {
			return c.Err(err.Error())
		}

		m.Publish = append(m.Publish, cfg)
	}

	return nil
}

func getStringFactory(s string) msgFactory {
	return func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
		rep := replacer.NewReplacer(ctx, req)
		return rep.Replace(s), nil
	}
}

func getExecCmdStringFactory(cmd []string) msgFactory {
	return func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
		args := make([]string, len(cmd))
		rep := replacer.NewReplacer(ctx, req)

		for i, c := range cmd {
			args[i] = rep.Replace(c)
		}

		output, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
		return string(output), err
	}
}

func parseConnectionSettings(cfg *PublishConfig, c *caddyfile.Dispenser) error {
	if cfg.Connection == nil {
		cfg.Connection = &ConnectionConfig{}
	}

	action := c.Val()
	if action == "clean-session" {
		cfg.Connection.CleanSession = true
		return nil
	}

	if !c.NextArg() {
		return c.ArgErr()
	}

	switch action {
	case "name":
		cfg.Name = c.Val()
	case "broker":
		cfg.Connection.Broker = append([]string{c.Val()}, c.RemainingArgs()...)
	case "user":
		cfg.Connection.User = c.Val()
	case "password":
		cfg.Connection.Password = c.Val()
	case "qos":
		i, err := strconv.Atoi(c.Val())
		if err != nil || i < 0 || i > 2 {
			return c.SyntaxErr("expected a number between 0 and 2")
		}
		cfg.Connection.QoS = i
	}

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*mqttPlugin)(nil)
	_ caddy.CleanerUpper    = (*mqttPlugin)(nil)
	_ caddyfile.Unmarshaler = (*mqttPlugin)(nil)
	_ plugin.Middleware     = (*mqttPlugin)(nil)
)
