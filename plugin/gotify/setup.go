package gotify

import (
	"context"

	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/core/replacer"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("gotify", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupGotify,
	})
}

func setupGotify(c *caddy.Controller) error {
	g, err := makeGotifyPlugin(c)
	if err != nil {
		return err
	}

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		g.next = next
		return g
	})

	return nil
}

func makeGotifyPlugin(c *caddy.Controller) (*gotifyPlugin, error) {
	g := &gotifyPlugin{}
	g.l = log.GetLogger(c, g)

	for c.Next() {
		var (
			msg   msgFactory
			title msgFactory
			srv   string
			token string
		)

		cond, err := matcher.SetupMatcherRemainingArgs(c)
		if err != nil {
			return nil, err
		}

		for c.NextBlock() {
			switch c.Val() {
			case "message", "m":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				m := c.Val()
				msg = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					g.l.Debugf("replacing message \"%s\"", m)

					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(m), nil
				}
			case "title", "t":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}

				t := c.Val()
				title = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					g.l.Debugf("replacing title \"%s\"", t)
					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(t), nil
				}

			case "server":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				srv = c.Val()

				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				token = c.Val()

			default:
				return nil, c.ArgErr()
			}
		}

		if srv == "" || token == "" {
			var ok bool
			srv, token, ok = g.findLastCreds()

			if !ok {
				return nil, c.Err("server keyword expected")
			}
		}

		if msg == nil && !cond.EmptyCondition() {
			return nil, c.Err("Message must not be empty if a condition is set")
		}

		n := &notification{
			Matcher: cond,
			msg:     msg,
			title:   title,
			srv:     srv,
			token:   token,
		}

		g.addNotification(n)
	}

	return g, nil
}
