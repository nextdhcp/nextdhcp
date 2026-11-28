package dhcpserver_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// logBuffer collects what zerolog logs. The servers log from the goroutines
// that serve requests.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

// messages returns the messages that were logged.
func (b *logBuffer) messages(t *testing.T) []map[string]any {
	t.Helper()

	b.mu.Lock()
	defer b.mu.Unlock()

	var messages []map[string]any

	scanner := bufio.NewScanner(bytes.NewReader(b.buf.Bytes()))
	for scanner.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &m), scanner.Text())
		messages = append(messages, m)
	}
	require.NoError(t, scanner.Err())

	return messages
}

// find returns the messages among messages whose text starts with prefix.
func find(messages []map[string]any, prefix string) []map[string]any {
	var found []map[string]any
	for _, m := range messages {
		if text, _ := m["message"].(string); strings.HasPrefix(text, prefix) {
			found = append(found, m)
		}
	}

	return found
}

// TestLogging runs Dhcpfiles with Caddy, like NextDHCP does, and checks that
// zerolog logs what the server, its handlers and Caddy log, from the level on
// that the log directive sets.
func TestLogging(t *testing.T) {
	const dhcpfile = `
127.0.0.1/8 {
	database memory
	lease 1h
	range 127.0.0.100 127.0.0.200
	%s
}
`

	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	// What is logged while a client gets an address. The handlers add the
	// fields of the request to what they log about it.
	var (
		received = map[string]any{
			"level":   "debug",
			"logger":  "dhcp",
			"message": "-> DISCOVER from 0.0.0.0:68 (client)",
		}
		found = map[string]any{
			"level":    "debug",
			"logger":   "dhcp.handlers.range",
			"hwaddr":   "00:aa:bb:cc:dd:ee",
			"xid":      "0x01020304",
			"secs":     float64(0),
			"msgtype":  "DISCOVER",
			"hostname": "client",
			"message":  "found unbound address for 00:aa:bb:cc:dd:ee: 127.0.0.100",
		}
		leased = map[string]any{
			"level":    "info",
			"logger":   "dhcp.handlers.range",
			"hwaddr":   "00:aa:bb:cc:dd:ee",
			"xid":      "0x01020304",
			"secs":     float64(0),
			"msgtype":  "REQUEST",
			"hostname": "client",
			"message":  "00:aa:bb:cc:dd:ee (binding): lease 127.0.0.100 for 1h0m0s (activeLeaseTime: 1h0m0s)",
		}
		caddyInfo = map[string]any{
			"level":   "info",
			"file":    "Dhcpfile",
			"message": "an info message of Caddy",
		}
		caddyError = map[string]any{
			"level":   "error",
			"error":   "failed",
			"message": "an error of Caddy",
		}
	)

	for name, tc := range map[string]struct {
		directive string
		logged    []map[string]any
		notLogged []map[string]any
	}{
		"debug": {
			directive: "log debug",
			logged:    []map[string]any{received, found, leased, caddyInfo, caddyError},
		},
		"default": {
			logged:    []map[string]any{leased, caddyInfo, caddyError},
			notLogged: []map[string]any{received, found},
		},
		"error": {
			directive: "log error",
			logged:    []map[string]any{caddyError},
			notLogged: []map[string]any{received, found, leased, caddyInfo},
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs := new(logBuffer)

			orig := log.Logger
			log.Logger = zerolog.New(logs)
			t.Cleanup(func() { log.Logger = orig })

			conn := newFakeConn()
			t.Cleanup(dhcpserver.SetListenDHCP(func(zerolog.Logger, net.IP, *net.Interface) (net.PacketConn, error) {
				return conn, nil
			}))

			cfgJSON, warnings, err := dhcpserver.Adapter{}.Adapt([]byte(strings.ReplaceAll(dhcpfile, "%s", tc.directive)), nil)
			require.NoError(t, err)
			require.Empty(t, warnings)

			require.NoError(t, caddy.Load(cfgJSON, true))

			stopped := false
			t.Cleanup(func() {
				if !stopped {
					_ = caddy.Stop()
				}
			})

			discover, err := dhcpv4.NewDiscovery(mac,
				dhcpv4.WithTransactionID(dhcpv4.TransactionID{0x01, 0x02, 0x03, 0x04}),
				dhcpv4.WithOption(dhcpv4.OptHostName("client")),
			)
			require.NoError(t, err)

			offer := conn.exchange(t, discover)
			require.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())

			request, err := dhcpv4.NewRequestFromOffer(offer, dhcpv4.WithOption(dhcpv4.OptHostName("client")))
			require.NoError(t, err)
			require.Equal(t, dhcpv4.MessageTypeAck, conn.exchange(t, request).MessageType())

			caddy.Log().Info("an info message of Caddy", zap.String("file", "Dhcpfile"))
			caddy.Log().Error("an error of Caddy", zap.Error(errors.New("failed")))

			// stopping waits until the requests are served, and logged
			require.NoError(t, caddy.Stop())
			stopped = true

			messages := logs.messages(t)

			for _, m := range tc.logged {
				assert.Equal(t, []map[string]any{m}, find(messages, m["message"].(string)))
			}

			for _, m := range tc.notLogged {
				assert.Empty(t, find(messages, m["message"].(string)))
			}

			// nothing else is logged from the error level on
			if name == "error" {
				assert.Len(t, messages, len(tc.logged), "%v", messages)
			}
		})
	}
}
