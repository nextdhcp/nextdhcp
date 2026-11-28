// Package log has the fields that NextDHCP adds to what it logs about a DHCP
// request. NextDHCP logs with zerolog.
package log

import (
	"context"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/rs/zerolog"
)

type requestFieldsKey struct{}

// requestFields are the fields that are logged about a DHCP request.
type requestFields struct {
	hwaddr   string
	xid      string
	secs     uint16
	msgtype  string
	hostname string
}

// AddRequestFields returns a new context.Context that has the given request assigned
func AddRequestFields(parent context.Context, req *dhcpv4.DHCPv4) context.Context {
	return context.WithValue(parent, requestFieldsKey{}, &requestFields{
		hwaddr:   req.ClientHWAddr.String(),
		xid:      req.TransactionID.String(),
		secs:     req.NumSeconds,
		msgtype:  req.MessageType().String(),
		hostname: req.HostName(),
	})
}

// With returns a logger that logs what parent does, with the fields of the
// request that is assigned to ctx, see AddRequestFields. It returns parent if
// no request is assigned to ctx.
func With(ctx context.Context, parent zerolog.Logger) zerolog.Logger {
	fields, ok := ctx.Value(requestFieldsKey{}).(*requestFields)
	if !ok {
		return parent
	}

	c := parent.With().
		Str("hwaddr", fields.hwaddr).
		Str("xid", fields.xid).
		Uint16("secs", fields.secs).
		Str("msgtype", fields.msgtype)

	if fields.hostname != "" {
		c = c.Str("hostname", fields.hostname)
	}

	return c.Logger()
}
