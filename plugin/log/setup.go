package log

import (
	"errors"
	"strings"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log/caddylog"
	"github.com/rs/zerolog"
)

func init() {
	dhcpserver.RegisterDirective("log", setupLogging)
}

// levels maps the log levels the log directive accepts to zerolog's log
// levels.
var levels = map[string]zerolog.Level{
	"debug":   zerolog.DebugLevel,
	"info":    zerolog.InfoLevel,
	"warn":    zerolog.WarnLevel,
	"warning": zerolog.WarnLevel,
	"error":   zerolog.ErrorLevel,
	"fatal":   zerolog.FatalLevel,
}

var errInvalidLevel = errors.New("invalid level")

func parseLevel(s string) (zerolog.Level, error) {
	l, ok := levels[strings.ToLower(s)]
	if !ok {
		return zerolog.NoLevel, errInvalidLevel
	}

	return l, nil
}

func setupLogging(c dhcpserver.Helper) error {
	c.Next()

	if !c.NextArg() {
		return c.ArgErr()
	}

	l, err := parseLevel(c.Val())
	if err != nil {
		return c.SyntaxErr(err.Error())
	}

	// TODO(ppacher): maybe we should allow log configuration like
	// 	log error to /tmp/error.log
	//	log debug to stdout
	//	...
	if c.Next() {
		return c.SyntaxErr("invalid token or multiple \"log\" configurations")
	}

	// The level applies to all logs, of all servers: to what NextDHCP logs
	// and to what Caddy logs
	caddylog.Configure(c.Config, l)

	return nil
}
