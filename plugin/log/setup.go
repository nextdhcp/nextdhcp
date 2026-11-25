package log

import (
	"errors"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
)

func init() {
	dhcpserver.RegisterDirective("log", setupLogging)
}

// levels maps the log levels the log directive accepts to Caddy's log
// levels.
var levels = map[string]string{
	"debug":   "DEBUG",
	"info":    "INFO",
	"warn":    "WARN",
	"warning": "WARN",
	"error":   "ERROR",
	"fatal":   "FATAL",
}

var errInvalidLevel = errors.New("invalid level")

func parseLevel(s string) (string, error) {
	l, ok := levels[strings.ToLower(s)]
	if !ok {
		return "", errInvalidLevel
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

	// The level applies to all logs, like the log level of NextDHCP did
	// before it used Caddy's logging
	defaultLog(c.Config).Level = l

	return nil
}

// defaultLog returns the configuration of Caddy's default log in cfg, which
// it adds if cfg doesn't have it yet.
func defaultLog(cfg *caddy.Config) *caddy.CustomLog {
	if cfg.Logging == nil {
		cfg.Logging = new(caddy.Logging)
	}

	if cfg.Logging.Logs == nil {
		cfg.Logging.Logs = make(map[string]*caddy.CustomLog)
	}

	l, ok := cfg.Logging.Logs[caddy.DefaultLoggerName]
	if !ok {
		l = new(caddy.CustomLog)
		cfg.Logging.Logs[caddy.DefaultLoggerName] = l
	}

	return l
}
