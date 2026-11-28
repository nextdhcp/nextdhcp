package log

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWith(t *testing.T) {
	newRequest := func(t *testing.T, modifiers ...dhcpv4.Modifier) *dhcpv4.DHCPv4 {
		t.Helper()

		req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}, append([]dhcpv4.Modifier{
			dhcpv4.WithTransactionID(dhcpv4.TransactionID{0x01, 0x02, 0x03, 0x04}),
		}, modifiers...)...)
		require.NoError(t, err)

		return req
	}

	t.Run("request", func(t *testing.T) {
		var buf bytes.Buffer
		parent := zerolog.New(&buf)

		req := newRequest(t, dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithOption(dhcpv4.OptHostName("client")))
		req.NumSeconds = 3

		l := With(AddRequestFields(context.Background(), req), parent)
		l.Info().Msg("a message")

		assert.JSONEq(t, `{
			"level": "info",
			"hwaddr": "00:aa:bb:cc:dd:ee",
			"xid": "0x01020304",
			"secs": 3,
			"msgtype": "REQUEST",
			"hostname": "client",
			"message": "a message"
		}`, buf.String())
	})

	t.Run("request without hostname", func(t *testing.T) {
		var buf bytes.Buffer

		// the fields of the request follow those of the parent
		parent := zerolog.New(&buf).With().Str("logger", "dhcp.handlers.range").Logger()

		l := With(AddRequestFields(context.Background(), newRequest(t)), parent)
		l.Warn().Str("ip", "10.0.0.1").Msg("a message")

		assert.Equal(t, `{"level":"warn","logger":"dhcp.handlers.range","hwaddr":"00:aa:bb:cc:dd:ee","xid":"0x01020304","secs":0,"msgtype":"DISCOVER","ip":"10.0.0.1","message":"a message"}`+"\n", buf.String())

		// the parent still logs without them
		buf.Reset()
		parent.Warn().Msg("a message")

		assert.Equal(t, `{"level":"warn","logger":"dhcp.handlers.range","message":"a message"}`+"\n", buf.String())
	})

	t.Run("no request", func(t *testing.T) {
		var buf bytes.Buffer
		parent := zerolog.New(&buf).With().Str("logger", "dhcp").Logger()

		l := With(context.Background(), parent)
		l.Info().Msg("a message")

		assert.Equal(t, `{"level":"info","logger":"dhcp","message":"a message"}`+"\n", buf.String())
	})

	t.Run("level of the parent", func(t *testing.T) {
		var buf bytes.Buffer
		parent := zerolog.New(&buf).Level(zerolog.WarnLevel)

		l := With(AddRequestFields(context.Background(), newRequest(t)), parent)
		l.Info().Msg("not logged")
		assert.Empty(t, buf.String())

		l.Warn().Msg("logged")
		assert.Contains(t, buf.String(), `"message":"logged"`)
	})

	// a logger that was never set up, like the one of a plugin that tests
	// create without provisioning it, logs nothing
	t.Run("zero logger", func(t *testing.T) {
		var parent zerolog.Logger

		l := With(AddRequestFields(context.Background(), newRequest(t)), parent)
		assert.NotPanics(t, func() { l.Error().Msg("not logged") })
	})
}
