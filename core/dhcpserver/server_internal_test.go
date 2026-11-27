package dhcpserver

import (
	"context"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/socket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestRawNakAddr checks that servers broadcast all NAKs through connections
// that read requests from *socket.Addr peers, like those of the packet
// transport, as RFC 2131 says, also to clients without an address that don't
// ask for broadcasts.
func TestRawNakAddr(t *testing.T) {
	srv := &Server{logger: zap.NewNop().Sugar(), IP: net.IPv4(10, 0, 0, 1)}
	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	for name, modifiers := range map[string][]dhcpv4.Modifier{
		"client without an address":       {},
		"client that asks for broadcasts": {dhcpv4.WithBroadcast(true)},
		"renewing client":                 {dhcpv4.WithClientIP(net.IPv4(10, 0, 0, 100))},
	} {
		t.Run(name, func(t *testing.T) {
			req, err := dhcpv4.New(append([]dhcpv4.Modifier{dhcpv4.WithHwAddr(mac), dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest)}, modifiers...)...)
			require.NoError(t, err)

			resp, err := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(dhcpv4.MessageTypeNak))
			require.NoError(t, err)

			peer := &socket.Addr{
				RawAddr: socket.RawAddr{MAC: mac, IP: req.ClientIPAddr},
				Local:   socket.RawAddr{IP: net.IPv4bcast},
			}

			got, ok := updateConnectionAddresses(context.Background(), peer, srv, req, resp).(*socket.Addr)
			require.True(t, ok)
			assert.Equal(t, "255.255.255.255", got.RawAddr.IP.String())
		})
	}
}

// TestUDPReplyAddr checks where servers send replies through connections
// that read requests from UDP addresses, like those of the udp transport,
// which can't send to the hardware address of a client.
func TestUDPReplyAddr(t *testing.T) {
	srv := &Server{logger: zap.NewNop().Sugar()}

	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	clientIP := net.IPv4(10, 0, 0, 100)
	relayIP := net.IPv4(10, 0, 1, 1)

	noAddress := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ClientPort}
	client := &net.UDPAddr{IP: clientIP, Port: dhcpv4.ClientPort}
	relay := &net.UDPAddr{IP: relayIP, Port: dhcpv4.ServerPort}
	broadcast := &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}

	cases := []struct {
		name  string
		req   []dhcpv4.Modifier
		reply dhcpv4.MessageType
		peer  *net.UDPAddr
		want  net.Addr
	}{
		{
			// RFC 2131 unicasts to the client's hardware address
			name:  "offer to a client without an address",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover)},
			reply: dhcpv4.MessageTypeOffer,
			peer:  noAddress,
			want:  broadcast,
		},
		{
			name:  "offer to a client that asks for broadcasts",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover), dhcpv4.WithBroadcast(true)},
			reply: dhcpv4.MessageTypeOffer,
			peer:  noAddress,
			want:  broadcast,
		},
		{
			name:  "ack to a client without an address",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest)},
			reply: dhcpv4.MessageTypeAck,
			peer:  noAddress,
			want:  broadcast,
		},
		{
			name:  "ack to a renewing client",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithClientIP(clientIP)},
			reply: dhcpv4.MessageTypeAck,
			peer:  client,
			want:  client,
		},
		{
			name:  "ack to an inform",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeInform), dhcpv4.WithClientIP(clientIP)},
			reply: dhcpv4.MessageTypeAck,
			peer:  client,
			want:  client,
		},
		{
			name:  "nak to a renewing client",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithClientIP(clientIP)},
			reply: dhcpv4.MessageTypeNak,
			peer:  client,
			want:  broadcast,
		},
		{
			name:  "nak to a client without an address",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest)},
			reply: dhcpv4.MessageTypeNak,
			peer:  noAddress,
			want:  broadcast,
		},
		{
			// relay agents are not supported yet
			name:  "relayed request",
			req:   []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover), dhcpv4.WithGatewayIP(relayIP)},
			reply: dhcpv4.MessageTypeOffer,
			peer:  relay,
			want:  relay,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := dhcpv4.New(append([]dhcpv4.Modifier{dhcpv4.WithHwAddr(mac)}, tc.req...)...)
			require.NoError(t, err)

			resp, err := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(tc.reply))
			require.NoError(t, err)

			if tc.reply != dhcpv4.MessageTypeNak {
				resp.YourIPAddr = clientIP
			}

			got := updateConnectionAddresses(context.Background(), tc.peer, srv, req, resp)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStartupInfo(t *testing.T) {
	servers := []*Server{
		{IP: net.IPv4(10, 0, 0, 1), Network: net.IPNet{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(24, 32)}, iface: net.Interface{Name: "eth0"}},
		{IP: net.IPv4(10, 0, 1, 1), Network: net.IPNet{IP: net.IPv4(10, 0, 1, 0), Mask: net.CIDRMask(24, 32)}, iface: net.Interface{Name: "eth1"}, Transport: TransportUDP},
	}

	assert.Equal(t, "Serving the following subnets\n"+
		"\t10.0.0.0/24 on 10.0.0.1 (eth0)\n"+
		"\t10.0.1.0/24 on 10.0.1.1 (eth1, udp transport)\n", getStartupInfo(servers))
}
