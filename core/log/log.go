package log

import (
	"context"

	"github.com/apex/log"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/logger"
)

type requestFieldsKey struct{}

// AddRequestFields returns a new context.Context that has the given request assigned
func AddRequestFields(parent context.Context, req *dhcpv4.DHCPv4) context.Context {
	fields := map[string]interface{}{
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

func With(ctx context.Context) {
	val := ctx.Value(requestFieldsKey{})
	if val != nil {
		if fields, ok := val.(log.Fields); ok {
			logger.AddDefaultFields(fields)
		}
	}
}
