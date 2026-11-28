package gotify

import (
	"context"
	"errors"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log/caddylog"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/core/replacer"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterModule(new(gotifyPlugin))
	dhcpserver.RegisterHandlerDirective("gotify", setupGotify)
}

// NotificationConfig configures a gotify notification.
type NotificationConfig struct {
	// Condition is the condition a request must match for the notification
	// to be sent. An empty condition matches all requests.
	Condition string `json:"condition,omitempty"`

	// Message is the message of the notification. Placeholders in it are
	// replaced with values of the request. No notification is sent if it is
	// not set.
	Message *string `json:"message,omitempty"`

	// Title is the title of the notification. Placeholders in it are
	// replaced with values of the request. It defaults to "NextDHCP".
	Title *string `json:"title,omitempty"`

	// Server is the URL of the gotify server.
	Server string `json:"server,omitempty"`

	// Token is the token of the gotify application.
	Token string `json:"token,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (*gotifyPlugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dhcp.handlers.gotify",
		New: func() caddy.Module { return new(gotifyPlugin) },
	}
}

// Provision sets up the configured notifications.
func (g *gotifyPlugin) Provision(ctx caddy.Context) error {
	g.l = caddylog.Logger(ctx)
	g.notifications = nil

	for _, cfg := range g.Notifications {
		n, err := g.makeNotification(cfg)
		if err != nil {
			return err
		}

		g.addNotification(n)
	}

	return nil
}

func (g *gotifyPlugin) makeNotification(cfg NotificationConfig) (*notification, error) {
	cond, err := matcher.SetupMatcherString(cfg.Condition)
	if err != nil {
		return nil, err
	}

	if cfg.Server == "" || cfg.Token == "" {
		return nil, errors.New("server keyword expected")
	}

	if cfg.Message == nil && !cond.EmptyCondition() {
		return nil, errors.New("message must not be empty if a condition is set")
	}

	n := &notification{
		Matcher: cond,
		srv:     cfg.Server,
		token:   cfg.Token,
	}

	if cfg.Message != nil {
		m := *cfg.Message
		n.msg = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
			g.l.Debug().Msgf("replacing message \"%s\"", m)

			rep := replacer.NewReplacer(ctx, req)
			return rep.Replace(m), nil
		}
	}

	if cfg.Title != nil {
		t := *cfg.Title
		n.title = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
			g.l.Debug().Msgf("replacing title \"%s\"", t)
			rep := replacer.NewReplacer(ctx, req)
			return rep.Replace(t), nil
		}
	}

	return n, nil
}

// Wrap implements plugin.Middleware.
func (g *gotifyPlugin) Wrap(next plugin.Handler) plugin.Handler {
	g.next = next
	return g
}

func setupGotify(h dhcpserver.Helper) (caddy.Module, error) {
	g := new(gotifyPlugin)
	return g, g.UnmarshalCaddyfile(h.Dispenser)
}

// UnmarshalCaddyfile sets up the plugin from all gotify directives of a
// server block.
func (g *gotifyPlugin) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	for c.Next() {
		var cfg NotificationConfig

		cfg.Condition = strings.Join(c.RemainingArgs(), " ")

		cond, err := matcher.SetupMatcherString(cfg.Condition)
		if err != nil {
			return err
		}

		for c.NextBlock(0) {
			switch c.Val() {
			case "message", "m":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m := c.Val()
				cfg.Message = &m
			case "title", "t":
				if !c.NextArg() {
					return c.ArgErr()
				}

				t := c.Val()
				cfg.Title = &t

			case "server":
				if !c.NextArg() {
					return c.ArgErr()
				}
				cfg.Server = c.Val()

				if !c.NextArg() {
					return c.ArgErr()
				}
				cfg.Token = c.Val()

			default:
				return c.ArgErr()
			}
		}

		if cfg.Server == "" || cfg.Token == "" {
			// use the credentials of the notification before
			if len(g.Notifications) == 0 {
				return c.Err("server keyword expected")
			}

			last := g.Notifications[len(g.Notifications)-1]
			cfg.Server, cfg.Token = last.Server, last.Token
		}

		if cfg.Message == nil && !cond.EmptyCondition() {
			return c.Err("message must not be empty if a condition is set")
		}

		g.Notifications = append(g.Notifications, cfg)
	}

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner     = (*gotifyPlugin)(nil)
	_ caddyfile.Unmarshaler = (*gotifyPlugin)(nil)
	_ plugin.Middleware     = (*gotifyPlugin)(nil)
)
