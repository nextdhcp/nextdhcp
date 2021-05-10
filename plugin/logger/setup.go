package logger

import (
	"github.com/caddyserver/caddy"
	"github.com/sirupsen/logrus"
)

func init() {
	Logger = logrus.New()
	initLog()
	caddy.RegisterPlugin("logger", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupLogging,
	})
	Log = logrus.NewEntry(Logger)
}

func setupLogging(c *caddy.Controller) error {
	if c.Next() {
		for c.NextBlock() {
			name := c.Val()
			values := c.RemainingArgs()
			if len(values) == 0 {
				return c.ArgErr()
			}
			parseLog(name, values)
		}
	}
	if c.Next() {
		return c.SyntaxErr("invalid token or multiple \"log\" configurations")
	}

	return nil
}
