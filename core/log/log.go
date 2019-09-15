package log

import (
	"github.com/apex/log"
	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/plugin"
)

// Logger is used by NextDHCP plugins to log important information
type Logger interface {
	// Debugf prints a debug message
	Debugf(msg string, args ...interface{})

	// Infof prints a info message
	Infof(msg string, args ...interface{})

	// Warnf prints a warning message
	Warnf(msg string, args ...interface{})

	// Errorf prints an error message
	Errorf(msg string, args ...interface{})
}

// GetLogger returns a new logger for the given controller and plugin
// plg may be nil in which case the server instance level logger is
// returned
func GetLogger(c *caddy.Controller, plg plugin.Handler) Logger {
	// TODO(ppacher): fix me
	if plg != nil {
		return log.WithField("plugin", plg.Name())
	}

	return log.Log
}
