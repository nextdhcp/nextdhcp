package socket

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/mdlayher/packet"
	"github.com/stretchr/testify/require"
)

type nopLogger struct{}

func (nopLogger) Debugf(string, ...interface{}) {}
func (nopLogger) Infof(string, ...interface{})  {}
func (nopLogger) Warnf(string, ...interface{})  {}
func (nopLogger) Errorf(string, ...interface{}) {}

// fakeRawConn stands in for the AF_PACKET socket. It records the frames
// written to it, and once closed its reads fail with an error that is not
// net.ErrClosed, as they do on a closed packet.Conn.
type fakeRawConn struct {
	closeOnce sync.Once
	closed    chan struct{}

	mu     sync.Mutex
	frames [][]byte
	addrs  []net.Addr
}

func newFakeRawConn() *fakeRawConn {
	return &fakeRawConn{closed: make(chan struct{})}
}

func (f *fakeRawConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-f.closed
	return 0, nil, errors.New("use of closed file")
}

func (f *fakeRawConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.frames = append(f.frames, append([]byte(nil), b...))
	f.addrs = append(f.addrs, addr)

	return len(b), nil
}

func (f *fakeRawConn) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeRawConn) LocalAddr() net.Addr              { return &packet.Addr{} }
func (f *fakeRawConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeRawConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeRawConn) SetWriteDeadline(time.Time) error { return nil }

// stubListeners replaces the socket constructors for the duration of the
// test. The UDP socket is an unprivileged one on 127.0.0.1, so no test needs
// port 67 or CAP_NET_RAW.
func stubListeners(t *testing.T, raw func(*net.Interface) (net.PacketConn, error)) {
	t.Helper()

	origUDP, origRaw := udpListenPacket, rawListenPacket
	t.Cleanup(func() {
		udpListenPacket, rawListenPacket = origUDP, origRaw
	})

	udpListenPacket = func(net.IP, int) (net.PacketConn, error) {
		return net.ListenPacket("udp4", "127.0.0.1:0")
	}
	rawListenPacket = raw
}

// listenWithFakeRaw returns a DHCPConn whose AF_PACKET socket is a
// fakeRawConn.
func listenWithFakeRaw(t *testing.T, ip net.IP, iface *net.Interface) (*DHCPConn, *fakeRawConn) {
	t.Helper()

	raw := newFakeRawConn()
	stubListeners(t, func(*net.Interface) (net.PacketConn, error) {
		return raw, nil
	})

	conn, err := ListenDHCP(nopLogger{}, ip, iface)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn.(*DHCPConn), raw
}

func TestDHCPConnCloseReturns(t *testing.T) {
	// Unprivileged UDP sockets stand in for the DHCP port and the AF_PACKET
	// socket, so the test needs neither port 67 nor CAP_NET_RAW.
	stubListeners(t, func(*net.Interface) (net.PacketConn, error) {
		return net.ListenPacket("udp4", "127.0.0.1:0")
	})

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

func TestListenDHCPClosesUDPOnRawError(t *testing.T) {
	var udp net.PacketConn

	stubListeners(t, nil)
	udpListenPacket = func(net.IP, int) (net.PacketConn, error) {
		var err error
		udp, err = net.ListenPacket("udp4", "127.0.0.1:0")
		return udp, err
	}

	rawErr := errors.New("raw listen failed")
	rawListenPacket = func(*net.Interface) (net.PacketConn, error) {
		return nil, rawErr
	}

	_, err := ListenDHCP(nopLogger{}, net.IPv4(127, 0, 0, 1), &net.Interface{Name: "test"})
	require.ErrorIs(t, err, rawErr)

	// Closing the UDP socket again only reports net.ErrClosed if ListenDHCP
	// already closed it.
	require.ErrorIs(t, udp.Close(), net.ErrClosed)
}

func TestDHCPConnWriteToRaw(t *testing.T) {
	serverMAC := net.HardwareAddr{0x02, 0, 0, 0, 0, 0x01}
	serverIP := net.IPv4(10, 0, 0, 1).To4()
	broadcastMAC := net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	clientMAC := net.HardwareAddr{0x02, 0, 0, 0, 0, 0xcc}
	clientIP := net.IPv4(10, 0, 0, 100).To4()

	msg, err := dhcpv4.NewDiscovery(clientMAC)
	require.NoError(t, err)
	payload := msg.ToBytes()

	cases := []struct {
		name       string
		ifaceMAC   net.HardwareAddr
		local      RawAddr
		wantSrcMAC net.HardwareAddr
		wantSrcIP  net.IP
	}{
		{
			name:       "request sent to us",
			ifaceMAC:   serverMAC,
			local:      RawAddr{MAC: serverMAC, IP: serverIP},
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			name:       "request sent to our IP with a broadcast MAC",
			ifaceMAC:   serverMAC,
			local:      RawAddr{MAC: broadcastMAC, IP: serverIP},
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			// e.g. a second server block on the same interface
			name:       "request sent to another server's address on our interface",
			ifaceMAC:   serverMAC,
			local:      RawAddr{MAC: serverMAC, IP: net.IPv4(10, 0, 0, 3).To4()},
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			name:       "request sent to a subnet-directed broadcast",
			ifaceMAC:   serverMAC,
			local:      RawAddr{MAC: broadcastMAC, IP: net.IPv4(10, 0, 0, 255).To4()},
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			name:       "request sent to another host",
			ifaceMAC:   serverMAC,
			local:      RawAddr{MAC: net.HardwareAddr{0x02, 0, 0, 0, 0, 0xbb}, IP: net.IPv4(10, 0, 0, 2).To4()},
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			name:       "no local address",
			ifaceMAC:   serverMAC,
			wantSrcMAC: serverMAC,
			wantSrcIP:  serverIP,
		},
		{
			name:       "interface without a hardware address",
			ifaceMAC:   nil,
			local:      RawAddr{MAC: net.HardwareAddr{0, 0, 0, 0, 0, 0}, IP: serverIP},
			wantSrcMAC: net.HardwareAddr{0, 0, 0, 0, 0, 0},
			wantSrcIP:  serverIP,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, raw := listenWithFakeRaw(t, serverIP, &net.Interface{Name: "test", HardwareAddr: tc.ifaceMAC})

			n, err := conn.WriteTo(payload, &Addr{
				RawAddr: RawAddr{MAC: clientMAC, IP: clientIP},
				Local:   tc.local,
			})
			require.NoError(t, err)
			require.Equal(t, len(payload), n, "WriteTo must report the bytes of b it sent, not the frame length")

			require.Len(t, raw.frames, 1)
			require.Equal(t, &packet.Addr{HardwareAddr: clientMAC}, raw.addrs[0])

			frame := gopacket.NewPacket(raw.frames[0], layers.LayerTypeEthernet, gopacket.Default)
			eth, ok := frame.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
			require.True(t, ok)
			ip, ok := frame.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			require.True(t, ok)

			require.Equal(t, tc.wantSrcMAC, eth.SrcMAC)
			require.Equal(t, clientMAC, eth.DstMAC)
			require.True(t, tc.wantSrcIP.Equal(ip.SrcIP), "source IP %s, want %s", ip.SrcIP, tc.wantSrcIP)
			require.True(t, clientIP.Equal(ip.DstIP), "destination IP %s, want %s", ip.DstIP, clientIP)
		})
	}
}

func TestDHCPConnReadFromAfterCloseReturnsErrClosed(t *testing.T) {
	conn, _ := listenWithFakeRaw(t, net.IPv4(127, 0, 0, 1), &net.Interface{Name: "test"})

	readErr := make(chan error, 1)
	go func() {
		_, _, err := conn.ReadFrom(make([]byte, 4096))
		readErr <- err
	}()

	require.NoError(t, conn.Close())

	select {
	case err := <-readErr:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFrom did not return within 5s after Close")
	}

	_, _, err := conn.ReadFrom(make([]byte, 4096))
	require.ErrorIs(t, err, net.ErrClosed)
}
