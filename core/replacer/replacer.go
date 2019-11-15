package replacer

import (
	"context"
	"net"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/option"
)

type (
	// Replacer is capable of replacing variables in a template string
	Replacer interface {
		// Replace replaces all variables in string and returns the result
		Replace(string) string

		// Set adds a custom replacement value
		Set(key string, value Value)

		// Get returns the replacement value for key
		Get(key string) string
	}

	// Value is a getter for string represenations of custom DHCPv4
	// fields
	Value interface {
		// Get returns the string represenation for the given
		// msg
		Get(msg *dhcpv4.DHCPv4) string
	}

	// ValueGetter implements the Value interface and returns a string
	// based on the provided DHCP message
	ValueGetter func(msg *dhcpv4.DHCPv4) string

	// StringValue is a utility method to use string constants for
	// the Value interface
	StringValue string

	// CtxKey is used to store a replace instance in a context value
	CtxKey struct{}

	replacer struct {
		msg                *dhcpv4.DHCPv4
		customReplacements map[string]Value // a list of custom replacements configured via Set
	}
)

// Get implements the Value interface and calls g itself
func (g ValueGetter) Get(m *dhcpv4.DHCPv4) string {
	return g(m)
}

// Get implements the Value interface and returns s itself
func (s StringValue) Get(_ *dhcpv4.DHCPv4) string {
	return string(s)
}

// WithReplacer returns a new context with a replacer instance
func WithReplacer(ctx context.Context, r Replacer) context.Context {
	return context.WithValue(ctx, CtxKey{}, r)
}

// GetReplacer returns the replacer associated with ctx
func GetReplacer(ctx context.Context) Replacer {
	v := ctx.Value(CtxKey{})
	if v == nil {
		return nil
	}

	r, ok := v.(Replacer)
	if !ok {
		panic("replacer.CtxKey used for a none replacer type")
	}
	return r
}

// NewReplacer returns a new replacer instance for the given
// request message
func NewReplacer(ctx context.Context, msg *dhcpv4.DHCPv4) Replacer {
	if parent := GetReplacer(ctx); parent != nil {
		// TODO(ppacher): how should we handle msg here?
		// we could add "parent lookup" support to child replacers
		// so any lookup bubble up the tree
		return parent
	}

	r := &replacer{
		msg:                msg,
		customReplacements: make(map[string]Value),
	}

	return r
}

func (r *replacer) Set(key string, val Value) {
	r.customReplacements[key] = val
}

// ipStr converts ip to it's string representation. If ip is nil or 0 bytes long
// an empty string is returned
func ipStr(ip net.IP) string {
	if ip == nil || len(ip) == 0 {
		return ""
	}

	return ip.String()
}

func (r *replacer) Get(key string) string {
	// try custom replacements first
	val, ok := r.customReplacements[key]
	if ok {
		return val.Get(r.msg)
	}

	// next check if we should lookup the value
	// from options
	if strings.HasPrefix(key, ">") {
		optName := strings.TrimPrefix(key, ">")
		code, ok := option.Code(optName)
		if !ok {
			return "<unknown>"
		}

		return option.ToString(code, r.msg.GetOneOption(code), nil)
	}

	// try built-in keys next
	switch key {
	case "msgtype":
		return r.msg.MessageType().String()

	case "yourip":
		return ipStr(r.msg.YourIPAddr)

	case "clientip":
		return ipStr(r.msg.ClientIPAddr)

	case "hwaddr":
		if r.msg.ClientHWAddr == nil {
			return ""
		}
		return r.msg.ClientHWAddr.String()

	case "requestedip":
		return ipStr(r.msg.RequestedIPAddress())

	case "hostname":
		return r.msg.HostName()

	case "gwip":
		return ipStr(r.msg.GatewayIPAddr)

	case "requested-options":
		if r.msg.ParameterRequestList() == nil {
			return ""
		}

		return r.msg.ParameterRequestList().String()

	case "state":
		return getClientState(r.msg)
	}

	// TODO(ppacher): should we make the "empty value" configurable
	return ""
}

func getClientState(msg *dhcpv4.DHCPv4) string {
	if msg.MessageType() == dhcpv4.MessageTypeDiscover {
		return "binding"
	}

	if msg.MessageType() == dhcpv4.MessageTypeRequest {
		if msg.ClientIPAddr != nil && !msg.ClientIPAddr.IsUnspecified() {
			return "renew"
		}

		if msg.RequestedIPAddress() != nil && !msg.RequestedIPAddress().IsUnspecified() {
			return "binding"
		}
	}

	return "unknown"
}

// Replace relaces all keys in s with their counterpart. The algorithm below
// is based and mostly copied from
// https://github.com/caddyserver/caddy/blob/master/caddyhttp/httpserver/replacer.go
func (r *replacer) Replace(s string) string {
	// Short path if no replacement keys are found
	if !strings.ContainsAny(s, "{}") {
		return s
	}

	// Do not attempt replacements if no placeholder is found.
	if !strings.ContainsAny(s, "{}") {
		return s
	}

	result := ""
Placeholders: // process each placeholder in sequence
	for {
		var idxStart, idxEnd int

		idxOffset := 0
		for { // find first unescaped opening brace
			searchSpace := s[idxOffset:]
			idxStart = strings.Index(searchSpace, "{")
			if idxStart == -1 {
				// no more placeholders
				break Placeholders
			}
			if idxStart == 0 || searchSpace[idxStart-1] != '\\' {
				// preceding character is not an escape
				idxStart += idxOffset
				break
			}
			// the brace we found was escaped
			// search the rest of the string next
			idxOffset += idxStart + 1
		}

		idxOffset = 0
		for { // find first unescaped closing brace
			searchSpace := s[idxStart+idxOffset:]
			idxEnd = strings.Index(searchSpace, "}")
			if idxEnd == -1 {
				// unpaired placeholder
				break Placeholders
			}
			if idxEnd == 0 || searchSpace[idxEnd-1] != '\\' {
				// preceding character is not an escape
				idxEnd += idxOffset + idxStart
				break
			}
			// the brace we found was escaped
			// search the rest of the string next
			idxOffset += idxEnd + 1
		}

		// get a replacement for the unescaped placeholder
		placeholder := unescapeBraces(s[idxStart : idxEnd+1])
		replacement := r.Get(placeholder[1 : len(placeholder)-1])

		// append unescaped prefix + replacement
		result += strings.TrimPrefix(unescapeBraces(s[:idxStart]), "\\") + replacement

		// strip out scanned parts
		s = s[idxEnd+1:]
	}

	// append unscanned parts
	return result + unescapeBraces(s)
}

// unescapeBraces finds escaped braces in s and returns
// a string with those braces unescaped.
func unescapeBraces(s string) string {
	s = strings.Replace(s, "\\{", "{", -1)
	s = strings.Replace(s, "\\}", "}", -1)
	return s
}
