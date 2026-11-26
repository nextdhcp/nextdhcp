package socket

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mdlayher/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	serverMAC = net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	deviceMAC = net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x02}
)

func TestARPRequest(t *testing.T) {
	// addresses parsed from text are in their 16-byte form
	frame, err := arpRequest(serverMAC, net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.10"))
	require.NoError(t, err)

	// Ethernet header and ARP packet with 6-byte hardware and 4-byte IPv4
	// addresses
	require.GreaterOrEqual(t, len(frame), 14+28)
	assert.Equal(t, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, frame[0:6])
	assert.Equal(t, []byte(serverMAC), frame[6:12])
	assert.Equal(t, []byte{0x08, 0x06}, frame[12:14])
	assert.Equal(t, []byte{0x00, 0x01, 0x08, 0x00, 6, 4, 0x00, 0x01}, frame[14:22])
	assert.Equal(t, []byte(serverMAC), frame[22:28])
	assert.Equal(t, []byte{10, 0, 0, 1}, frame[28:32])
	assert.Equal(t, make([]byte, 6), frame[32:38])
	assert.Equal(t, []byte{10, 0, 0, 10}, frame[38:42])

	// the frame comes from the server
	sender, ok := arpSender(frame)
	require.True(t, ok)
	assert.Equal(t, net.IP{10, 0, 0, 1}, sender)
}

func TestARPRequestInvalid(t *testing.T) {
	_, err := arpRequest(serverMAC, net.ParseIP("10.0.0.1"), net.ParseIP("fd00::1"))
	assert.Error(t, err)

	_, err = arpRequest(serverMAC, net.ParseIP("fd00::1"), net.ParseIP("10.0.0.10"))
	assert.Error(t, err)

	// interfaces without an Ethernet address, like tunnels
	_, err = arpRequest(nil, net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.10"))
	assert.Error(t, err)
}

// arpFrame returns an Ethernet frame with an ARP packet from the device with
// the addresses mac and ip.
func arpFrame(t *testing.T, operation uint16, mac net.HardwareAddr, ip net.IP) []byte {
	t.Helper()

	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true},
		&layers.Ethernet{SrcMAC: mac, DstMAC: serverMAC, EthernetType: layers.EthernetTypeARP},
		&layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			Operation:         operation,
			SourceHwAddress:   mac,
			SourceProtAddress: ip.To4(),
			DstHwAddress:      serverMAC,
			DstProtAddress:    net.IP{10, 0, 0, 1},
		},
	))

	return buf.Bytes()
}

func TestARPSender(t *testing.T) {
	sender, ok := arpSender(arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 10}))
	require.True(t, ok)
	assert.Equal(t, net.IP{10, 0, 0, 10}, sender)

	// not ARP
	udp, err := PreparePacket(deviceMAC, net.IP{10, 0, 0, 10}, serverMAC, net.IP{10, 0, 0, 1}, []byte("payload"))
	require.NoError(t, err)
	_, ok = arpSender(udp)
	assert.False(t, ok)

	// truncated
	_, ok = arpSender(arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 10})[:30])
	assert.False(t, ok)

	_, ok = arpSender(nil)
	assert.False(t, ok)
}

// arpRead is what a read from a fakeARPConn returns.
type arpRead struct {
	frame []byte
	err   error
}

// deadline is the read of a fakeARPConn whose read deadline passes.
var deadline = arpRead{err: &net.OpError{Op: "read", Net: "packet", Err: os.ErrDeadlineExceeded}}

// fakeARPConn stands in for the ARP socket. Its reads return reads, in order,
// and then pass their deadline.
type fakeARPConn struct {
	mu       sync.Mutex
	reads    []arpRead
	writeErr error
	written  [][]byte
	to       []net.Addr
	closed   bool
}

func (c *fakeARPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	read := deadline
	if len(c.reads) > 0 {
		read, c.reads = c.reads[0], c.reads[1:]
	}

	return copy(b, read.frame), &packet.Addr{}, read.err
}

func (c *fakeARPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writeErr != nil {
		return 0, c.writeErr
	}

	c.written = append(c.written, append([]byte(nil), b...))
	c.to = append(c.to, addr)

	return len(b), nil
}

func (c *fakeARPConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return nil
}

func (c *fakeARPConn) LocalAddr() net.Addr              { return &packet.Addr{} }
func (c *fakeARPConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeARPConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeARPConn) SetWriteDeadline(time.Time) error { return nil }

// stubARPListener makes ProbeARP use conn, and returns the interfaces it was
// opened on.
func stubARPListener(t *testing.T, conn net.PacketConn, err error) *[]string {
	t.Helper()

	orig := arpListenPacket
	t.Cleanup(func() { arpListenPacket = orig })

	var opened []string
	arpListenPacket = func(iface *net.Interface) (net.PacketConn, error) {
		opened = append(opened, iface.Name)
		return conn, err
	}

	return &opened
}

func TestProbeARP(t *testing.T) {
	ctx := context.Background()
	iface := &net.Interface{Name: "eth0", HardwareAddr: serverMAC}
	serverIP, ip := net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.10")

	request, err := arpRequest(serverMAC, serverIP, ip)
	require.NoError(t, err)

	t.Run("answered", func(t *testing.T) {
		conn := &fakeARPConn{reads: []arpRead{
			{frame: request}, // our own request
			{frame: arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 11})},
			{frame: []byte("not a frame")},
			{frame: arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 10})},
		}}
		opened := stubARPListener(t, conn, nil)

		used, err := ProbeARP(ctx, iface, serverIP, ip)
		require.NoError(t, err)
		assert.True(t, used)

		assert.Equal(t, []string{"eth0"}, *opened)
		assert.Equal(t, [][]byte{request}, conn.written)
		assert.Equal(t, []net.Addr{&packet.Addr{HardwareAddr: ethernetBroadcast}}, conn.to)
		assert.True(t, conn.closed)
	})

	t.Run("answered after a lost request", func(t *testing.T) {
		conn := &fakeARPConn{reads: []arpRead{
			deadline,
			{frame: arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 10})},
		}}
		stubARPListener(t, conn, nil)

		used, err := ProbeARP(ctx, iface, serverIP, ip)
		require.NoError(t, err)
		assert.True(t, used)
		assert.Len(t, conn.written, 2)
	})

	t.Run("announced", func(t *testing.T) {
		// a device that asks for another address from ip uses it too
		conn := &fakeARPConn{reads: []arpRead{
			{frame: arpFrame(t, layers.ARPRequest, deviceMAC, net.IP{10, 0, 0, 10})},
		}}
		stubARPListener(t, conn, nil)

		used, err := ProbeARP(ctx, iface, serverIP, ip)
		require.NoError(t, err)
		assert.True(t, used)
	})

	t.Run("silent", func(t *testing.T) {
		conn := &fakeARPConn{reads: []arpRead{
			{frame: request},
			{frame: arpFrame(t, layers.ARPReply, deviceMAC, net.IP{10, 0, 0, 11})},
		}}
		stubARPListener(t, conn, nil)

		used, err := ProbeARP(ctx, iface, serverIP, ip)
		require.NoError(t, err)
		assert.False(t, used)

		// each request got its own time to be answered
		assert.Equal(t, [][]byte{request, request, request}, conn.written)
		assert.True(t, conn.closed)
	})

	t.Run("read error", func(t *testing.T) {
		conn := &fakeARPConn{reads: []arpRead{{err: errors.New("read failed")}}}
		stubARPListener(t, conn, nil)

		_, err := ProbeARP(ctx, iface, serverIP, ip)
		assert.ErrorContains(t, err, "read failed")
		assert.True(t, conn.closed)
	})

	t.Run("write error", func(t *testing.T) {
		conn := &fakeARPConn{writeErr: errors.New("write failed")}
		stubARPListener(t, conn, nil)

		_, err := ProbeARP(ctx, iface, serverIP, ip)
		assert.ErrorContains(t, err, "write failed")
	})

	t.Run("listen error", func(t *testing.T) {
		stubARPListener(t, nil, errors.New("operation not permitted"))

		_, err := ProbeARP(ctx, iface, serverIP, ip)
		assert.ErrorContains(t, err, "operation not permitted")
	})

	t.Run("canceled", func(t *testing.T) {
		conn := &fakeARPConn{}
		stubARPListener(t, conn, nil)

		ctx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := ProbeARP(ctx, iface, serverIP, ip)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, conn.written)
	})

	t.Run("canceled while waiting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// the context is canceled while ProbeARP waits for answers to its
		// last request, so it doesn't decide that no device uses ip
		conn := &cancelingARPConn{fakeARPConn: &fakeARPConn{}, cancelAfter: arpRequests, cancel: cancel}
		stubARPListener(t, conn, nil)

		_, err := ProbeARP(ctx, iface, serverIP, ip)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("not IPv4", func(t *testing.T) {
		opened := stubARPListener(t, &fakeARPConn{}, nil)

		_, err := ProbeARP(ctx, iface, serverIP, net.ParseIP("fd00::1"))
		assert.Error(t, err)
		assert.Empty(t, *opened)
	})
}

// cancelingARPConn is a fakeARPConn that calls cancel once cancelAfter
// requests have been written to it.
type cancelingARPConn struct {
	*fakeARPConn
	cancelAfter int
	cancel      context.CancelFunc
}

func (c *cancelingARPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	n, err := c.fakeARPConn.WriteTo(b, addr)

	c.mu.Lock()
	written := len(c.written)
	c.mu.Unlock()

	if written == c.cancelAfter {
		c.cancel()
	}

	return n, err
}
