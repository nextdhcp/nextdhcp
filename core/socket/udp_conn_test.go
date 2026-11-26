package socket

import (
	"errors"
	"net"
	"os"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

var (
	eth0 = &net.Interface{Index: 2, Name: "eth0"}
	eth1 = &net.Interface{Index: 3, Name: "eth1"}
)

// udpRead is what a read from a fakeUDPSocket returns.
type udpRead struct {
	payload []byte
	cm      *ipv4.ControlMessage
	src     net.Addr
	err     error
}

// udpWrite is a datagram written to a fakeUDPSocket.
type udpWrite struct {
	payload []byte
	cm      *ipv4.ControlMessage
	dst     net.Addr
}

// fakeUDPSocket stands in for the socket that the connections of ListenUDP
// share. Its reads return what is sent to reads, until it is closed.
type fakeUDPSocket struct {
	reads chan udpRead

	closeOnce sync.Once
	closed    chan struct{}

	mu     sync.Mutex
	writes []udpWrite
}

func newFakeUDPSocket() *fakeUDPSocket {
	return &fakeUDPSocket{reads: make(chan udpRead), closed: make(chan struct{})}
}

func (s *fakeUDPSocket) ReadFrom(b []byte) (int, *ipv4.ControlMessage, net.Addr, error) {
	select {
	case r := <-s.reads:
		return copy(b, r.payload), r.cm, r.src, r.err
	case <-s.closed:
		return 0, nil, nil, &net.OpError{Op: "read", Net: "udp4", Err: net.ErrClosed}
	}
}

func (s *fakeUDPSocket) WriteTo(b []byte, cm *ipv4.ControlMessage, dst net.Addr) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writes = append(s.writes, udpWrite{payload: append([]byte(nil), b...), cm: cm, dst: dst})

	return len(b), nil
}

func (s *fakeUDPSocket) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4zero, Port: 67}
}

func (s *fakeUDPSocket) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeUDPSocket) isClosed() bool {
	return isClosed(s.closed)
}

func (s *fakeUDPSocket) written() []udpWrite {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]udpWrite(nil), s.writes...)
}

// stubUDPListen makes ListenUDP open fake sockets, and returns the sockets
// it opened. It checks that the test closed all connections.
func stubUDPListen(t *testing.T) *[]*fakeUDPSocket {
	t.Helper()

	orig := udpListen
	var opened []*fakeUDPSocket
	udpListen = func(string) (udpSocket, error) {
		s := newFakeUDPSocket()
		opened = append(opened, s)
		return s, nil
	}

	t.Cleanup(func() {
		udpListen = orig

		udpMu.Lock()
		defer udpMu.Unlock()

		if udpShared != nil {
			t.Error("the shared UDP socket is still open")
			_ = udpShared.stopLocked()
		}
	})

	return &opened
}

// listenUDP returns a connection of ListenUDP, which is closed when the test
// ends.
func listenUDP(t *testing.T, ip string, iface *net.Interface) *udpConn {
	t.Helper()

	c, err := ListenUDP(nopLogger{}, net.ParseIP(ip), iface)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	return c.(*udpConn)
}

// requireRead reads the next request from c.
func requireRead(t *testing.T, c net.PacketConn) ([]byte, net.Addr) {
	t.Helper()

	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	defer func() { require.NoError(t, c.SetReadDeadline(time.Time{})) }()

	buf := make([]byte, 1500)
	n, addr, err := c.ReadFrom(buf)
	require.NoError(t, err)

	return buf[:n], addr
}

func TestListenUDPDispatch(t *testing.T) {
	stubUDPListen(t)

	a := listenUDP(t, "10.0.0.1", eth0)
	b := listenUDP(t, "10.0.1.1", eth1)
	c := listenUDP(t, "10.0.0.2", eth0) // e.g. a second subnet on eth0

	client := &net.UDPAddr{IP: net.IPv4zero, Port: 68}

	cases := []struct {
		name string
		cm   *ipv4.ControlMessage
		want []*udpConn
	}{
		{
			name: "broadcast on eth0",
			cm:   &ipv4.ControlMessage{IfIndex: eth0.Index, Dst: net.IPv4bcast},
			want: []*udpConn{a, c},
		},
		{
			name: "broadcast on eth1",
			cm:   &ipv4.ControlMessage{IfIndex: eth1.Index, Dst: net.IPv4bcast},
			want: []*udpConn{b},
		},
		{
			name: "sent to the address of a server",
			cm:   &ipv4.ControlMessage{IfIndex: eth0.Index, Dst: net.IPv4(10, 0, 0, 2)},
			want: []*udpConn{c},
		},
		{
			name: "sent to another address of the interface",
			cm:   &ipv4.ControlMessage{IfIndex: eth0.Index, Dst: net.IPv4(10, 0, 0, 9)},
			want: []*udpConn{a, c},
		},
		{
			name: "sent to the address of a server on another interface",
			cm:   &ipv4.ControlMessage{IfIndex: eth1.Index, Dst: net.IPv4(10, 0, 0, 1)},
			want: []*udpConn{b},
		},
		{
			name: "interface without a server",
			cm:   &ipv4.ControlMessage{IfIndex: 9, Dst: net.IPv4bcast},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a.listener.dispatch([]byte("request"), tc.cm, client)

			for _, conn := range []*udpConn{a, b, c} {
				if !slices.Contains(tc.want, conn) {
					assert.Empty(t, conn.in, "the server %s on %s got the request", conn.ip, conn.iface.Name)
					continue
				}

				payload, src := requireRead(t, conn)
				assert.Equal(t, []byte("request"), payload)
				assert.Equal(t, client, src)
				assert.NotSame(t, client, src, "servers must not share the client's address")
			}
		})
	}
}

func TestListenUDPRead(t *testing.T) {
	opened := stubUDPListen(t)
	conn := listenUDP(t, "10.0.0.1", eth0)
	sock := (*opened)[0]

	client := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 100), Port: 68}
	cm := &ipv4.ControlMessage{IfIndex: eth0.Index, Dst: net.IPv4(10, 0, 0, 1)}

	sock.reads <- udpRead{payload: []byte("request"), cm: cm, src: client}
	payload, src := requireRead(t, conn)
	assert.Equal(t, []byte("request"), payload)
	assert.Equal(t, client, src)

	// Without the interface, or a payload, there is no request to serve
	sock.reads <- udpRead{payload: []byte("unknown interface"), src: client}
	sock.reads <- udpRead{cm: cm, src: client}

	// A request that doesn't fit in the buffer is cut off, as UDP sockets do
	sock.reads <- udpRead{payload: []byte("too long"), cm: cm, src: client}

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 3)
	n, _, err := conn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "too", string(buf[:n]))
}

func TestListenUDPWriteTo(t *testing.T) {
	opened := stubUDPListen(t)
	conn := listenUDP(t, "10.0.0.1", eth0)
	sock := (*opened)[0]

	broadcast := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	n, err := conn.WriteTo([]byte("offer"), broadcast)
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	client := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 100), Port: 68}
	n, err = conn.WriteTo([]byte("ack"), client)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	// Replies come from the server's address, and broadcasts leave through
	// its interface
	assert.Equal(t, []udpWrite{
		{payload: []byte("offer"), cm: &ipv4.ControlMessage{Src: net.IP{10, 0, 0, 1}, IfIndex: eth0.Index}, dst: broadcast},
		{payload: []byte("ack"), cm: &ipv4.ControlMessage{Src: net.IP{10, 0, 0, 1}}, dst: client},
	}, sock.written())

	for _, addr := range []net.Addr{
		&net.IPAddr{IP: net.IPv4(10, 0, 0, 100)},
		&net.UDPAddr{IP: net.ParseIP("fd00::1"), Port: 68},
	} {
		_, err := conn.WriteTo([]byte("ack"), addr)
		assert.Error(t, err, "%s", addr)
	}

	assert.Len(t, sock.written(), 2)
}

func TestListenUDPSharesSocket(t *testing.T) {
	opened := stubUDPListen(t)

	a, err := ListenUDP(nopLogger{}, net.ParseIP("10.0.0.1"), eth0)
	require.NoError(t, err)
	b, err := ListenUDP(nopLogger{}, net.ParseIP("10.0.1.1"), eth1)
	require.NoError(t, err)
	require.Len(t, *opened, 1)

	// The socket stays open as long as a connection uses it
	require.NoError(t, a.Close())
	assert.False(t, (*opened)[0].isClosed())

	require.NoError(t, b.Close())
	assert.True(t, (*opened)[0].isClosed())

	// The next connection opens a new socket
	c, err := ListenUDP(nopLogger{}, net.ParseIP("10.0.0.1"), eth0)
	require.NoError(t, err)
	assert.Len(t, *opened, 2)
	require.NoError(t, c.Close())
}

func TestListenUDPClose(t *testing.T) {
	stubUDPListen(t)
	conn := listenUDP(t, "10.0.0.1", eth0)

	readErr := make(chan error, 1)
	go func() {
		_, _, err := conn.ReadFrom(make([]byte, 1500))
		readErr <- err
	}()

	require.NoError(t, conn.Close())

	select {
	case err := <-readErr:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFrom did not return within 5s after Close")
	}

	_, _, err := conn.ReadFrom(make([]byte, 1500))
	assert.ErrorIs(t, err, net.ErrClosed)

	_, err = conn.WriteTo([]byte("offer"), &net.UDPAddr{IP: net.IPv4bcast, Port: 68})
	assert.ErrorIs(t, err, net.ErrClosed)

	assert.ErrorIs(t, conn.Close(), net.ErrClosed)
}

func TestListenUDPReadFailure(t *testing.T) {
	opened := stubUDPListen(t)
	a := listenUDP(t, "10.0.0.1", eth0)
	b := listenUDP(t, "10.0.1.1", eth1)
	sock := (*opened)[0]

	sock.reads <- udpRead{err: errors.New("socket failed")}

	// Reading fails for all servers, with an error that doesn't look like
	// they were closed or timed out, so they stop serving
	for _, conn := range []*udpConn{a, b} {
		_, _, err := conn.ReadFrom(make([]byte, 1500))
		require.ErrorContains(t, err, "socket failed")
		assert.NotErrorIs(t, err, net.ErrClosed)

		var netErr net.Error
		require.ErrorAs(t, err, &netErr)
		assert.False(t, netErr.Timeout())
	}

	assert.True(t, sock.isClosed())

	// New connections open a new socket, while the failed ones are still
	// open
	c := listenUDP(t, "10.0.2.1", eth0)
	require.Len(t, *opened, 2)
	assert.NotSame(t, a.listener, c.listener)
}

func TestListenUDPInterfaceGone(t *testing.T) {
	origList, origInterval := listInterfaces, interfaceCheckInterval
	t.Cleanup(func() { listInterfaces, interfaceCheckInterval = origList, origInterval })

	listInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{*eth1}, nil
	}
	interfaceCheckInterval = 10 * time.Millisecond

	opened := stubUDPListen(t)
	a := listenUDP(t, "10.0.0.1", eth0)
	b := listenUDP(t, "10.0.1.1", eth1)

	require.NoError(t, a.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err := a.ReadFrom(make([]byte, 1500))
	assert.ErrorContains(t, err, "network interface eth0 went away")

	// The servers on other interfaces keep serving
	(*opened)[0].reads <- udpRead{
		payload: []byte("request"),
		cm:      &ipv4.ControlMessage{IfIndex: eth1.Index, Dst: net.IPv4bcast},
		src:     &net.UDPAddr{IP: net.IPv4zero, Port: 68},
	}
	payload, _ := requireRead(t, b)
	assert.Equal(t, []byte("request"), payload)
	assert.False(t, (*opened)[0].isClosed())
}

func TestListenUDPDeadlines(t *testing.T) {
	opened := stubUDPListen(t)
	conn := listenUDP(t, "10.0.0.1", eth0)
	buf := make([]byte, 1500)

	// A deadline that passed
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(-time.Second)))
	_, _, err := conn.ReadFrom(buf)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout())

	// A deadline that passes while reading
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(20*time.Millisecond)))
	_, _, err = conn.ReadFrom(buf)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	// Without a deadline, reading waits for the next request
	require.NoError(t, conn.SetReadDeadline(time.Time{}))
	go func() {
		(*opened)[0].reads <- udpRead{
			payload: []byte("request"),
			cm:      &ipv4.ControlMessage{IfIndex: eth0.Index, Dst: net.IPv4bcast},
			src:     &net.UDPAddr{IP: net.IPv4zero, Port: 68},
		}
	}()

	n, _, err := conn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "request", string(buf[:n]))

	broadcast := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(-time.Second)))
	_, err = conn.WriteTo([]byte("offer"), broadcast)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	require.NoError(t, conn.SetDeadline(time.Time{}))
	_, err = conn.WriteTo([]byte("offer"), broadcast)
	require.NoError(t, err)
}

func TestListenUDPErrors(t *testing.T) {
	t.Run("not IPv4", func(t *testing.T) {
		opened := stubUDPListen(t)

		_, err := ListenUDP(nopLogger{}, net.ParseIP("fd00::1"), eth0)
		assert.Error(t, err)
		assert.Empty(t, *opened)
	})

	t.Run("listen error", func(t *testing.T) {
		orig := udpListen
		t.Cleanup(func() { udpListen = orig })

		listenErr := errors.New("address already in use")
		udpListen = func(string) (udpSocket, error) { return nil, listenErr }

		_, err := ListenUDP(nopLogger{}, net.ParseIP("10.0.0.1"), eth0)
		assert.ErrorIs(t, err, listenErr)

		udpMu.Lock()
		defer udpMu.Unlock()
		assert.Nil(t, udpShared)
	})
}

// TestListenUDPLoopback exchanges datagrams with a client through a real
// socket, on the loopback interface and an unprivileged port.
func TestListenUDPLoopback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("golang.org/x/net/ipv4 doesn't report the interface of datagrams on Windows")
	}

	lo := loopbackInterface(t)

	origAddr := udpListenAddr
	t.Cleanup(func() { udpListenAddr = origAddr })
	udpListenAddr = "127.0.0.1:0"

	conn, err := ListenUDP(nopLogger{}, net.IPv4(127, 0, 0, 1), lo)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	server, ok := conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)
	require.Equal(t, "127.0.0.1", server.IP.String())
	require.NotZero(t, server.Port)

	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.WriteTo([]byte("request"), server)
	require.NoError(t, err)

	payload, src := requireRead(t, conn)
	assert.Equal(t, "request", string(payload))
	assert.Equal(t, client.LocalAddr().String(), src.String())

	n, err := conn.WriteTo([]byte("reply"), src)
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1500)
	n, from, err := client.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "reply", string(buf[:n]))
	assert.Equal(t, server.String(), from.String())
}

// loopbackInterface returns the loopback interface, or skips the test if
// there is none.
func loopbackInterface(t *testing.T) *net.Interface {
	t.Helper()

	ifaces, err := net.Interfaces()
	require.NoError(t, err)

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 && ifc.Flags&net.FlagUp != 0 {
			return &ifc
		}
	}

	t.Skip("no loopback interface is up")
	return nil
}
