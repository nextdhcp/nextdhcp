package socket

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type nopLogger struct{}

func (nopLogger) Debugf(string, ...interface{}) {}
func (nopLogger) Infof(string, ...interface{})  {}
func (nopLogger) Warnf(string, ...interface{})  {}
func (nopLogger) Errorf(string, ...interface{}) {}

func TestDHCPConnCloseReturns(t *testing.T) {
	origUDP, origRaw := udpListenPacket, rawListenPacket
	t.Cleanup(func() {
		udpListenPacket, rawListenPacket = origUDP, origRaw
	})

	// Unprivileged UDP sockets stand in for the DHCP port and the AF_PACKET
	// socket, so the test needs neither port 67 nor CAP_NET_RAW.
	udpListenPacket = func(net.IP, int) (net.PacketConn, error) {
		return net.ListenPacket("udp4", "127.0.0.1:0")
	}
	rawListenPacket = func(*net.Interface) (net.PacketConn, error) {
		return net.ListenPacket("udp4", "127.0.0.1:0")
	}

	conn, err := ListenDHCP(nopLogger{}, net.IPv4(127, 0, 0, 1), &net.Interface{Name: "test"})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- conn.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return within 5s")
	}
}
