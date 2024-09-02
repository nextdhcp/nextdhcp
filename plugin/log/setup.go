package log

import (
	"os"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	"github.com/caddyserver/caddy"
	"github.com/mattn/go-isatty"
)

func init() {
	caddy.RegisterPlugin("log", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupLogging,
	})
}

func setupLogging(c *caddy.Controller) error {
	c.Next()

	if !c.NextArg() {
		return c.ArgErr()
	}

	l, err := log.ParseLevel(c.Val())
	if err != nil {
		return c.SyntaxErr(err.Error())
	}

	log.SetLevel(l)

	if isatty.IsTerminal(os.Stdout.Fd()) {
		log.SetHandler(cli.New(os.Stdout))
	}

	// TODO(ppacher): maybe we should allow log configuration like
	// 	log error to /tmp/error.log
	//	log debug to stdout
	//	...
	if c.Next() {
		return c.SyntaxErr("invalid token or multiple \"log\" configurations")
	}

	return nil
}
