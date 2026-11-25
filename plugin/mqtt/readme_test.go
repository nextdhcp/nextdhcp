package mqtt

import (
	"context"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadmeExamples(t *testing.T) {
	examples := test.Examples(t, "README.md")
	require.Len(t, examples, 2)

	for _, example := range examples {
		_, err := makeMqttPlugin(t, example)
		assert.NoError(t, err, example)
	}

	// the first example builds a JSON payload
	m, err := makeMqttPlugin(t, examples[0])
	require.NoError(t, err)
	require.Len(t, m.configs, 2)

	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithClientIP(net.IP{10, 1, 0, 5}),
	)
	require.NoError(t, err)

	match, err := m.configs[0].Match(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, match)

	payload, err := m.configs[1].payload(context.Background(), req, nil)
	require.NoError(t, err)
	assert.Equal(t, `{"clientIP": "10.1.0.5"}`, payload)
}
