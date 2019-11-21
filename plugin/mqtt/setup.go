package mqtt

import (
	"context"
	"os/exec"
	"strconv"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/core/replacer"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("mqtt", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupMqtt,
	})
}

func setupMqtt(c *caddy.Controller) error {
	plg := &mqttPlugin{}
	plg.l = log.GetLogger(c, plg)

	for c.Next() {
		cfg := &mqttConfig{}
		useExisting := false

		setConn := func() error {
			if useExisting {
				return c.SyntaxErr("either configure a new connection or \"use\" and existing one")
			}

			cfg.conn = &mqttConnConfig{}

			return nil
		}

		cond, err := matcher.SetupMatcherRemainingArgs(c)
		if err != nil {
			return err
		}
		cfg.Matcher = cond

		for c.NextBlock() {
			switch c.Val() {
			case "name", "broker", "user", "password",
				"clean-session", "qos":
				action := c.Val()
				if err := setConn(); err != nil {
					return err
				}

				if action == "clean-session" {
					cfg.conn.cleanSession = true
					break
				}

				if !c.NextArg() {
					return c.ArgErr()
				}

				switch action {
				case "name":
					cfg.name = c.Val()
				case "broker":
					cfg.conn.broker = append([]string{c.Val()}, c.RemainingArgs()...)
				case "user":
					cfg.conn.user = c.Val()
				case "password":
					cfg.conn.password = c.Val()
				case "qos":
					i, err := strconv.Atoi(c.Val())
					if err != nil || i < 0 || i > 2 {
						return c.SyntaxErr("expected a number between 0 and 2")
					}
					cfg.conn.qos = i
				}

			case "use":
				if cfg.conn != nil {
					return c.SyntaxErr("either configure a new connection or \"use\" and existing one")
				}
				useExisting = true

				if !c.NextArg() {
					return c.ArgErr()
				}
				cfg.name = c.Val()

			case "topic":
				if !c.NextArg() {
					return c.ArgErr()
				}

				t := c.Val()
				cfg.topic = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(t), nil
				}

			case "payload", "body":
				if !c.NextArg() {
					return c.ArgErr()
				}

				t := c.Val()
				cfg.payload = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(t), nil
				}

			case "payload-from":
				//
				// TODO(ppacher): payload-from allows to execute an external script and use it's output
				// to publish on MQTT. Is this really required? We could also just provide an "exec" plugin
				// that calls an external binary/script and use that for publishing to MQTT.
				//
				plg.l.Warnf("payload-from: use of unofficial directive detected")
				plg.l.Warnf("payload-from: this directive may vanish in future versions.")

				cmd := c.RemainingArgs()
				if len(cmd) == 0 {
					return c.ArgErr()
				}

				cfg.payload = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					args := make([]string, len(cmd))
					rep := replacer.NewReplacer(ctx, req)

					for i, c := range cmd {
						args[i] = rep.Replace(c)
					}

					output, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
					return string(output), err
				}
			}
		}

		if !useExisting && cfg.conn == nil {
			return c.SyntaxErr("Either configure a MQTT connection or \"use\" an existing one")
		}

		plg.configs = append(plg.configs, cfg)
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.next = next
		return plg
	})
	return nil
}
