package gotify

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
	require.Len(t, examples, 1)

	g, err := makeGotifyPlugin(t, examples[0])
	require.NoError(t, err)
	require.Len(t, g.notifications, 1)

	n := g.notifications[0]
	assert.Equal(t, "https://gotify.example.com", n.srv)
	assert.Equal(t, "Adk57Vkdh487", n.token)

	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptHostName("laptop")),
		dhcpv4.WithClientIP(net.IP{10, 1, 0, 5}),
	)
	require.NoError(t, err)
	res, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	title, msg, err := n.Prepare(context.Background(), req, res)
	require.NoError(t, err)
	assert.Equal(t, "REQUEST by laptop", title)
	assert.Equal(t, "Client 00:aa:bb:cc:dd:ee", msg)
}
