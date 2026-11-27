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

// TestUpdateConnectionAddresses checks where responses to clients on the
// served subnet are sent, as RFC 2131 section 4.1 says.
func TestUpdateConnectionAddresses(t *testing.T) {
	var (
		clientMAC = net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
		serverMAC = net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		serverIP  = net.IP{10, 0, 0, 1}
		ciaddr    = net.IP{10, 0, 0, 50}
		yiaddr    = net.IP{10, 0, 0, 60}
		broadcast = net.IP{255, 255, 255, 255}
	)

	srv := &Server{
		IP:     serverIP,
		iface:  net.Interface{Name: "eth0", HardwareAddr: serverMAC},
		logger: zap.NewNop().Sugar(),
	}

	for name, tc := range map[string]struct {
		response  dhcpv4.MessageType
		ciaddr    net.IP
		broadcast bool // the broadcast bit of the request
		to        net.IP
	}{
		"offer": {
			response: dhcpv4.MessageTypeOffer,
			to:       yiaddr,
		},
		"offer with broadcast bit": {
			response:  dhcpv4.MessageTypeOffer,
			broadcast: true,
			to:        broadcast,
		},
		"ack": {
			response: dhcpv4.MessageTypeAck,
			to:       yiaddr,
		},
		"ack with broadcast bit": {
			response:  dhcpv4.MessageTypeAck,
			broadcast: true,
			to:        broadcast,
		},
		"ack of renewal": {
			response: dhcpv4.MessageTypeAck,
			ciaddr:   ciaddr,
			to:       ciaddr,
		},
		"ack of renewal with broadcast bit": {
			response:  dhcpv4.MessageTypeAck,
			ciaddr:    ciaddr,
			broadcast: true,
			to:        ciaddr,
		},
		// a client in INIT-REBOOT or SELECTING state has no address yet
		"nak": {
			response: dhcpv4.MessageTypeNak,
			to:       broadcast,
		},
		"nak with broadcast bit": {
			response:  dhcpv4.MessageTypeNak,
			broadcast: true,
			to:        broadcast,
		},
		"nak of renewal": {
			response: dhcpv4.MessageTypeNak,
			ciaddr:   ciaddr,
			to:       broadcast,
		},
	} {
		t.Run(name, func(t *testing.T) {
			modifiers := []dhcpv4.Modifier{
				dhcpv4.WithHwAddr(clientMAC),
				dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			}
			if tc.ciaddr != nil {
				modifiers = append(modifiers, dhcpv4.WithClientIP(tc.ciaddr))
			}
			if tc.broadcast {
				modifiers = append(modifiers, dhcpv4.WithBroadcast(true))
			}

			req, err := dhcpv4.New(modifiers...)
			require.NoError(t, err)

			resp, err := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(tc.response))
			require.NoError(t, err)

			if tc.response != dhcpv4.MessageTypeNak {
				resp.YourIPAddr = yiaddr
			}

			// the request was broadcast from the client, which has no
			// address yet unless it renews
			srcIP := net.IPv4zero.To4()
			if tc.ciaddr != nil {
				srcIP = tc.ciaddr
			}

			addr := &socket.Addr{
				RawAddr: socket.RawAddr{MAC: clientMAC, IP: srcIP, Port: dhcpv4.ClientPort},
				Local:   socket.RawAddr{MAC: net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, IP: broadcast, Port: dhcpv4.ServerPort},
			}

			to, ok := updateConnectionAddresses(context.Background(), addr, srv, req, resp).(*socket.Addr)
			require.True(t, ok)

			assert.Equal(t, tc.to.String(), to.IP.String(), "destination IP")
			assert.Equal(t, clientMAC, to.MAC, "destination MAC")
			assert.Equal(t, serverIP.String(), to.Local.IP.String(), "source IP")
			assert.Equal(t, serverMAC, to.Local.MAC, "source MAC")
		})
	}
}
