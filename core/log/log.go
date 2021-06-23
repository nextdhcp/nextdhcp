package log

import (
	"context"

	"github.com/apex/log"
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type requestFieldsKey struct{}

// AddRequestFields returns a new context.Context that has the given request assigned
func AddRequestFields(parent context.Context, req *dhcpv4.DHCPv4) context.Context {
	fields := log.Fields{
		"hwaddr":  req.ClientHWAddr.String(),
		"xid":     req.TransactionID,
		"secs":    req.NumSeconds,
		"msgtype": req.MessageType().String(),
	}

	if req.HostName() != "" {
		fields["hostname"] = req.HostName()
	}

	return context.WithValue(parent, requestFieldsKey{}, fields)
}

// With add parent log fields to current log field
func With(ctx context.Context, parent Logger) Logger {
	l, ok := parent.(log.Interface)
	if !ok {
		return parent
	}

	val := ctx.Value(requestFieldsKey{})
	if val != nil {
		if fields, ok := val.(log.Fields); ok {
			return l.WithFields(fields)
		}
	}

	return l
}

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
func GetLogger(c *caddy.Controller, plg plugin.Handler) log.Interface {
	// TODO(ppacher): fix me
	if plg != nil {
		return log.WithField("plugin", plg.Name())
	}

	return log.Log
}
