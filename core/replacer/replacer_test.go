package replacer

import (
	"context"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
)

func Test_Replacer_Context_Utils(t *testing.T) {
	t.Run("WithReplacer should add it to the context", func(t *testing.T) {
		ctx := context.Background()

		r := &replacer{}
		ctx = WithReplacer(ctx, r)

		fromCtx := ctx.Value(CtxKey{})
		assert.NotNil(t, fromCtx)
		assert.Exactly(t, r, fromCtx)
	})

	t.Run("GetReplacer should return it from a context", func(t *testing.T) {
		ctx := context.Background()

		r := &replacer{}
		ctx = context.WithValue(ctx, CtxKey{}, r)

		fromCtx := ctx.Value(CtxKey{})
		assert.NotNil(t, fromCtx)
		assert.Exactly(t, r, GetReplacer(ctx))
	})

	t.Run("GetReplacer should return nil if not in a context", func(t *testing.T) {
		assert.Nil(t, GetReplacer(context.Background()))
	})

	t.Run("GetReplacer should panic if key is misused", func(t *testing.T) {
		assert.Panics(t, func() {
			GetReplacer(context.WithValue(context.Background(), CtxKey{}, "foobar"))
		})
	})
}

func Test_Replacer_KnownKeys(t *testing.T) {
	msg, err := dhcpv4.NewDiscovery(net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02})
	if err != nil {
		panic(err)
	}

	msg.YourIPAddr = net.IP{10, 0, 0, 1}
	msg.ClientIPAddr = net.IP{10, 0, 0, 2}
	msg.UpdateOption(dhcpv4.OptRequestedIPAddress(net.IP{10, 0, 0, 3}))
	msg.UpdateOption(dhcpv4.OptHostName("host"))
	msg.GatewayIPAddr = net.IP{10, 0, 0, 4}

	r := NewReplacer(context.Background(), msg)

	t.Run("simple keys and options", func(t *testing.T) {
		cases := []struct {
			I string
			E string
		}{
			{
				"msgtype",
				"DISCOVER",
			},
			{
				"yourip",
				"10.0.0.1",
			},
			{
				"clientip",
				"10.0.0.2",
			},
			{
				"hwaddr",
				"de:ad:be:ef:01:02",
			},
			{
				"requestedip",
				"10.0.0.3",
			},
			{
				"hostname",
				"host",
			},
			{
				"gwip",
				"10.0.0.4",
			},
			{
				">hostname",
				"host",
			},
			{
				">unknown-option",
				"<unknown>",
			},
		}

		for i, c := range cases {
			res := r.Get(c.I)
			assert.Equal(t, c.E, res, "in case %d", i)
		}
	})

	t.Run("msgtype", func(t *testing.T) {
		cases := []struct {
			I dhcpv4.MessageType
			E string
		}{
			{
				dhcpv4.MessageTypeAck,
				"ACK",
			},
			{
				dhcpv4.MessageTypeDecline,
				"DECLINE",
			},
			{
				dhcpv4.MessageTypeDiscover,
				"DISCOVER",
			},
			{
				dhcpv4.MessageTypeInform,
				"INFORM",
			},
			{
				dhcpv4.MessageTypeNak,
				"NAK",
			},
			{
				dhcpv4.MessageTypeOffer,
				"OFFER",
			},
			{
				dhcpv4.MessageTypeRelease,
				"RELEASE",
			},
			{
				dhcpv4.MessageTypeRequest,
				"REQUEST",
			},
		}

		for i, c := range cases {
			msg.UpdateOption(dhcpv4.OptMessageType(c.I))
			res := r.Get("msgtype")
			assert.Equal(t, c.E, res, "in case %d", i)
		}
	})

	t.Run("state", func(t *testing.T) {
		cases := []struct {
			I func()
			E string
		}{
			// DISCOVER is always in "binding" state
			{
				func() {
					msg.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover))
				},
				"binding",
			},
			// REQUEST is "renew" with a ClientIP
			{
				func() {
					msg.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest))
					msg.ClientIPAddr = net.IP{10, 0, 0, 1}
				},
				"renew",
			},
			// REQUEST is "binding" with requested IP
			{
				func() {
					msg.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest))
					msg.UpdateOption(dhcpv4.OptRequestedIPAddress(net.IP{10, 0, 0, 1}))
					msg.ClientIPAddr = nil
				},
				"binding",
			},
			// Other types are unknown
			{
				func() {
					msg.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeInform))
				},
				"unknown",
			},
			// REQUEST is "unknown" without requested IP or client IP
			{
				func() {
					msg.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest))
					msg.UpdateOption(dhcpv4.OptRequestedIPAddress(nil))
					msg.ClientIPAddr = nil
				},
				"unknown",
			},
		}

		for i, c := range cases {
			c.I()
			res := r.Get("state")
			assert.Equal(t, c.E, res, "in case %d", i)
		}
	})

	t.Run("custom keys", func(t *testing.T) {
		r.Set("key1", StringValue("value1"))
		assert.Equal(t, "value1", r.Get("key1"))

		r.Set("foo", getter(func() string {
			return "bar"
		}))

		assert.Equal(t, "bar", r.Get("foo"))

		r.Set("msgtype", ValueGetter(func(m *dhcpv4.DHCPv4) string {
			assert.Exactly(t, msg, m)
			return "msgtype"
		}))

		assert.Equal(t, "msgtype", r.Get("msgtype"))
	})
}

func Test_Replacer_Replace(t *testing.T) {
	msg, err := dhcpv4.NewDiscovery(net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02})
	if err != nil {
		panic(err)
	}

	msg.YourIPAddr = net.IP{10, 0, 0, 1}
	msg.ClientIPAddr = net.IP{10, 0, 0, 2}
	msg.UpdateOption(dhcpv4.OptRequestedIPAddress(net.IP{10, 0, 0, 3}))
	msg.UpdateOption(dhcpv4.OptHostName("host"))
	msg.UpdateOption(dhcpv4.OptRouter(net.IP{10, 0, 0, 254}))
	msg.GatewayIPAddr = net.IP{10, 0, 0, 4}

	r := NewReplacer(context.Background(), msg)

	cases := []struct {
		I string
		E string
	}{
		{
			"{hostname} {hwaddr} requested {requestedip}",
			"host de:ad:be:ef:01:02 requested 10.0.0.3",
		},
		{
			"\\{hostname} {hwaddr} requested {requestedip}",
			"{hostname} de:ad:be:ef:01:02 requested 10.0.0.3",
		},
		{
			"\\{hostname\\} {hwaddr} requested {requestedip}",
			"{hostname} de:ad:be:ef:01:02 requested 10.0.0.3",
		},
		{
			"{hostname\\} {hwaddr} requested {requestedip}",
			" requested 10.0.0.3",
		},
		{
			"router is {>router}",
			"router is 10.0.0.254",
		},
		{
			"{",
			"{",
		},
		{
			"{}",
			"",
		},
		{
			"}",
			"}",
		},
	}

	for i, c := range cases {
		assert.Equal(t, c.E, r.Replace(c.I), "in case %d", i)
	}
}

type getter func() string

func (g getter) Get(_ *dhcpv4.DHCPv4) string {
	return g()
}
