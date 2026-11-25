package log

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
)

type requestFieldsKey struct{}

// AddRequestFields returns a new context.Context that has the given request assigned
func AddRequestFields(parent context.Context, req *dhcpv4.DHCPv4) context.Context {
	fields := []any{
		"hwaddr", req.ClientHWAddr.String(),
		"xid", req.TransactionID.String(),
		"secs", req.NumSeconds,
		"msgtype", req.MessageType().String(),
	}

	if req.HostName() != "" {
		fields = append(fields, "hostname", req.HostName())
	}

	return context.WithValue(parent, requestFieldsKey{}, fields)
}

// With add field in ctx to log
func With(ctx context.Context, parent Logger) Logger {
	l, ok := parent.(*zap.SugaredLogger)
	if !ok {
		return parent
	}

	val := ctx.Value(requestFieldsKey{})
	if val != nil {
		if fields, ok := val.([]any); ok {
			return l.With(fields...)
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
