package log

import (
	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/plugin"
	"github.com/sirupsen/logrus"
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
	return logrus.New()
}
