package transport

import (
	"testing"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTransport(t *testing.T) {
	for _, transport := range []string{dhcpserver.TransportPacket, dhcpserver.TransportUDP} {
		t.Run(transport, func(t *testing.T) {
			c := test.CreateTestBed(t, "transport "+transport)

			require.NoError(t, setupTransport(c))
			assert.Equal(t, transport, c.Server.Transport)
		})
	}

	for name, input := range map[string]string{
		"no transport":      "transport",
		"unknown transport": "transport raw",
		"two transports":    "transport udp packet",
		"set twice":         "transport udp\ntransport packet",
	} {
		t.Run(name, func(t *testing.T) {
			c := test.CreateTestBed(t, input)

			assert.Error(t, setupTransport(c))
		})
	}
}
