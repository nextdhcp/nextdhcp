package dhcpserver_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type packet struct {
	payload []byte
	addr    net.Addr
}

// fakeConn is a net.PacketConn that tests write requests to and read
// responses from.
type fakeConn struct {
	requests  chan packet
	responses chan packet
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		requests:  make(chan packet),
		responses: make(chan packet, 16),
		closed:    make(chan struct{}),
	}
}

func (c *fakeConn) ReadFrom(b []byte) (int, net.Addr, error) {
	select {
	case p := <-c.requests:
		return copy(b, p.payload), p.addr, nil
	case <-c.closed:
		return 0, nil, &net.OpError{Op: "read", Net: "fake", Err: net.ErrClosed}
	}
}

func (c *fakeConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case c.responses <- packet{payload: append([]byte(nil), b...), addr: addr}:
		return len(b), nil
	}
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr                { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 67} }
func (c *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

// exchange sends req to the server and returns its response.
func (c *fakeConn) exchange(t *testing.T, req *dhcpv4.DHCPv4) *dhcpv4.DHCPv4 {
	t.Helper()

	client := &net.UDPAddr{IP: net.IPv4zero, Port: dhcpv4.ClientPort}

	select {
	case c.requests <- packet{payload: req.ToBytes(), addr: client}:
	case <-time.After(5 * time.Second):
		t.Fatal("server didn't read the request")
	}

	select {
	case p := <-c.responses:
		resp, err := dhcpv4.FromBytes(p.payload)
		require.NoError(t, err)
		return resp
	case <-time.After(5 * time.Second):
		t.Fatal("server didn't respond")
		return nil
	}
}

const servingDhcpfile = `
127.0.0.1/8 {
	database memory
	option router 127.0.0.1
	servername nextdhcp
	next-server 127.0.0.2
	lease 1h
	range 127.0.0.100 127.0.0.200
}
`

// TestServe serves DISCOVER and REQUEST messages with an app adapted from a
// Dhcpfile, through a connection that doesn't need raw socket privileges.
func TestServe(t *testing.T) {
	conn := newFakeConn()
	restore := dhcpserver.SetListenDHCP(func(_ log.Logger, ip net.IP, ifc *net.Interface) (net.PacketConn, error) {
		assert.Equal(t, "127.0.0.1", ip.String())
		assert.Equal(t, "lo", ifc.Name)
		return conn, nil
	})
	defer restore()

	_, app, err := adapt(t, servingDhcpfile)
	require.NoError(t, err)

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	require.NoError(t, app.Provision(ctx))
	defer func() { assert.NoError(t, app.Cleanup()) }()

	require.NoError(t, app.Start())

	stopped := false
	defer func() {
		if !stopped {
			_ = app.Stop()
		}
	}()

	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	discover, err := dhcpv4.NewDiscovery(mac, dhcpv4.WithRequestedOptions(
		dhcpv4.OptionSubnetMask,
		dhcpv4.OptionRouter,
	))
	require.NoError(t, err)

	offer := conn.exchange(t, discover)
	assert.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())
	assert.Equal(t, discover.TransactionID, offer.TransactionID)
	assert.Equal(t, "127.0.0.1", offer.ServerIdentifier().String())
	assert.Equal(t, "127.0.0.100", offer.YourIPAddr.String())
	assert.Equal(t, "255.0.0.0", net.IP(offer.SubnetMask()).String())
	assert.Equal(t, time.Hour, offer.IPAddressLeaseTime(0))
	require.Len(t, offer.Router(), 1)
	assert.Equal(t, "127.0.0.1", offer.Router()[0].String())
	assert.Equal(t, "nextdhcp", offer.ServerHostName)
	assert.Equal(t, "127.0.0.2", offer.ServerIPAddr.String())

	request, err := dhcpv4.NewRequestFromOffer(offer)
	require.NoError(t, err)

	ack := conn.exchange(t, request)
	assert.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
	assert.Equal(t, "127.0.0.100", ack.YourIPAddr.String())
	assert.Equal(t, time.Hour, ack.IPAddressLeaseTime(0))

	// another client gets the next address
	discover, err = dhcpv4.NewDiscovery(net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xff})
	require.NoError(t, err)

	offer = conn.exchange(t, discover)
	assert.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())
	assert.Equal(t, "127.0.0.101", offer.YourIPAddr.String())

	// Stop returns once the server stopped serving
	done := make(chan error, 1)
	go func() { done <- app.Stop() }()

	select {
	case err := <-done:
		stopped = true
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop didn't return")
	}
}

// TestStartFailure checks that Start stops the servers it already started if
// a later one fails to start.
func TestStartFailure(t *testing.T) {
	first := newFakeConn()
	calls := 0
	restore := dhcpserver.SetListenDHCP(func(log.Logger, net.IP, *net.Interface) (net.PacketConn, error) {
		calls++
		if calls == 1 {
			return first, nil
		}

		return nil, assert.AnError
	})
	defer restore()

	_, app, err := adapt(t, `
127.0.0.1/8 {
	database memory
}

lo {
	database memory
}
`)
	require.NoError(t, err)

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	require.NoError(t, app.Provision(ctx))
	defer func() { assert.NoError(t, app.Cleanup()) }()

	assert.ErrorIs(t, app.Start(), assert.AnError)

	select {
	case <-first.closed:
	default:
		t.Error("the server that started wasn't stopped")
	}
}

// TestSharedDatabaseFile checks that two servers configured with the same
// bolt database file fail to start instead of waiting forever for its lock.
func TestSharedDatabaseFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "leases.db")

	cfg, _, err := adapt(t, fmt.Sprintf(`
127.0.0.1/8 {
	database bolt {
		file %s
		timeout 100ms
	}
}

lo {
	database bolt {
		file %s
		timeout 100ms
	}
}
`, file, file))
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- caddy.Validate(cfg) }()

	select {
	case err := <-done:
		assert.ErrorContains(t, err, "locked")
	case <-time.After(10 * time.Second):
		t.Fatal("provisioning the servers doesn't return")
	}
}
