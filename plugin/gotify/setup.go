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
			return err
		}

		for c.NextBlock() {
			switch c.Val() {
			case "message", "m":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m := c.Val()
				msg = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					g.l.Debugf("replacing message \"%s\"", m)

					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(m), nil
				}
			case "title", "t":
				if !c.NextArg() {
					return c.ArgErr()
				}

				t := c.Val()
				title = func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					g.l.Debugf("replacing title \"%s\"", t)
					rep := replacer.NewReplacer(ctx, req)
					return rep.Replace(t), nil
				}

			case "server":
				if !c.NextArg() {
					return c.ArgErr()
				}
				srv = c.Val()

				if !c.NextArg() {
					return c.ArgErr()
				}
				token = c.Val()
			default:
				return c.ArgErr()
			}
		}

		if srv == "" || token == "" {
			return c.Err("server keyword expected")
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

	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		g.next = next
		return g
	})

	return nil
}
