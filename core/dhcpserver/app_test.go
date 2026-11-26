package dhcpserver_test

import (
	"context"
	"errors"
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
	readErrs  chan error // errors for ReadFrom to return
	closed    chan struct{}
	closeErr  error // returned by ReadFrom once closed, instead of net.ErrClosed
	closeOnce sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		requests:  make(chan packet),
		responses: make(chan packet, 16),
		readErrs:  make(chan error),
		closed:    make(chan struct{}),
	}
}

func (c *fakeConn) ReadFrom(b []byte) (int, net.Addr, error) {
	select {
	case p := <-c.requests:
		return copy(b, p.payload), p.addr, nil
	case err := <-c.readErrs:
		return 0, nil, err
	case <-c.closed:
		if c.closeErr != nil {
			return 0, nil, c.closeErr
		}

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

// bind sends a DISCOVER for client and a REQUEST for the address the server
// offers, and returns the offer and the server's response to the request.
func (c *fakeConn) bind(t *testing.T, client net.HardwareAddr) (offer, ack *dhcpv4.DHCPv4) {
	t.Helper()

	discover, err := dhcpv4.NewDiscovery(client)
	require.NoError(t, err)

	offer = c.exchange(t, discover)
	request, err := dhcpv4.NewRequestFromOffer(offer)
	require.NoError(t, err)

	return offer, c.exchange(t, request)
}

// TestServe serves DISCOVER and REQUEST messages with an app adapted from a
// Dhcpfile, through a connection that doesn't need raw socket privileges.
func TestServe(t *testing.T) {
	conn := newFakeConn()
	app := provisionApp(t, servingDhcpfile, func(_ log.Logger, ip net.IP, ifc *net.Interface) (net.PacketConn, error) {
		assert.Equal(t, "127.0.0.1", ip.String())
		assert.Equal(t, "lo", ifc.Name)
		return conn, nil
	})

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

// provisionApp provisions an app adapted from dhcpfile, whose servers open
// their connection with listen. The app is cleaned up when the test ends.
func provisionApp(t *testing.T, dhcpfile string, listen func(log.Logger, net.IP, *net.Interface) (net.PacketConn, error)) *dhcpserver.App {
	t.Helper()

	t.Cleanup(dhcpserver.SetListenDHCP(listen))

	_, app, err := adapt(t, dhcpfile)
	require.NoError(t, err)

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	t.Cleanup(cancel)

	require.NoError(t, app.Provision(ctx))
	t.Cleanup(func() { assert.NoError(t, app.Cleanup()) })

	return app
}

// startApp provisions and starts an app adapted from dhcpfile, which must
// configure a single server, and returns the connection the server serves.
// The app is stopped and cleaned up when the test ends.
func startApp(t *testing.T, dhcpfile string) *fakeConn {
	t.Helper()

	conn := newFakeConn()
	startAppWithConn(t, dhcpfile, conn)

	return conn
}

// startAppWithConn is like startApp, but its server serves conn, and it
// returns the app.
func startAppWithConn(t *testing.T, dhcpfile string, conn *fakeConn) *dhcpserver.App {
	t.Helper()

	app := provisionApp(t, dhcpfile, func(log.Logger, net.IP, *net.Interface) (net.PacketConn, error) {
		return conn, nil
	})

	require.NoError(t, app.Start())
	t.Cleanup(func() { assert.NoError(t, app.Stop()) })

	return app
}

// TestStaticAddressReserved checks that an address the static plugin assigns
// to a client is not leased to other clients, even if it's part of a range.
func TestStaticAddressReserved(t *testing.T) {
	const (
		dhcpfile = `
127.0.0.1/8 {
	database %s
	range 127.0.0.10 127.0.0.20
	%s
}
`
		static = "static aa:bb:cc:dd:ee:ff 127.0.0.10"
	)

	staticClient := net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	otherClient := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}
	thirdClient := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x02}

	// leaseAddress checks that client gets ip with a DISCOVER and a REQUEST.
	leaseAddress := func(t *testing.T, conn *fakeConn, client net.HardwareAddr, ip string) {
		t.Helper()

		offer, ack := conn.bind(t, client)
		assert.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())
		assert.Equal(t, ip, offer.YourIPAddr.String())
		assert.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
		assert.Equal(t, ip, ack.YourIPAddr.String())
	}

	t.Run("discover", func(t *testing.T) {
		conn := startApp(t, fmt.Sprintf(dhcpfile, "memory", static))

		// other clients get the next address of the range, even if they ask
		// for the static one
		discover, err := dhcpv4.NewDiscovery(otherClient,
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 10})),
		)
		require.NoError(t, err)

		offer := conn.exchange(t, discover)
		assert.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())
		assert.Equal(t, "127.0.0.11", offer.YourIPAddr.String())

		leaseAddress(t, conn, otherClient, "127.0.0.11")
		leaseAddress(t, conn, staticClient, "127.0.0.10")
	})

	t.Run("request", func(t *testing.T) {
		conn := startApp(t, fmt.Sprintf(dhcpfile, "memory", static))

		// other clients can't request the static address
		request, err := dhcpv4.NewDiscovery(otherClient,
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP{127, 0, 0, 10})),
		)
		require.NoError(t, err)

		nak := conn.exchange(t, request)
		assert.Equal(t, dhcpv4.MessageTypeNak, nak.MessageType())

		leaseAddress(t, conn, staticClient, "127.0.0.10")
	})

	t.Run("leased before", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		// another client leases the address before it is assigned to a
		// client
		require.True(t, t.Run("without static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			leaseAddress(t, conn, otherClient, "127.0.0.10")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, static))

		// its lease can't be renewed
		renew, err := dhcpv4.New(
			dhcpv4.WithHwAddr(otherClient),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithClientIP(net.IP{127, 0, 0, 10}),
		)
		require.NoError(t, err)

		nak := conn.exchange(t, renew)
		assert.Equal(t, dhcpv4.MessageTypeNak, nak.MessageType())

		// so it gets a new address
		leaseAddress(t, conn, otherClient, "127.0.0.11")
		leaseAddress(t, conn, staticClient, "127.0.0.10")
	})

	t.Run("static client leased another address before", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("without static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			leaseAddress(t, conn, otherClient, "127.0.0.10")
			leaseAddress(t, conn, staticClient, "127.0.0.11")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, static))

		// the client uses its lease until it tries to renew it, so other
		// clients don't get that address
		leaseAddress(t, conn, thirdClient, "127.0.0.12")
	})

	t.Run("failed start", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("lease", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			leaseAddress(t, conn, otherClient, "127.0.0.10")
		}))

		// NextDHCP fails to start with the address assigned to a client
		require.True(t, t.Run("static", func(t *testing.T) {
			app := provisionApp(t, fmt.Sprintf(dhcpfile, database, static), func(log.Logger, net.IP, *net.Interface) (net.PacketConn, error) {
				return nil, assert.AnError
			})
			assert.ErrorIs(t, app.Start(), assert.AnError)
		}))

		// so the lease of the other client is kept
		conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
		leaseAddress(t, conn, thirdClient, "127.0.0.11")
	})
}

// TestLeaseTimeOfKeptLease checks that a client that requests its lease
// again without renewing it, like after a reboot, is told the time that
// remains of the lease, which is when the database lets it expire.
func TestLeaseTimeOfKeptLease(t *testing.T) {
	conn := startApp(t, servingDhcpfile)
	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	_, ack := conn.bind(t, mac)
	require.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
	assert.Equal(t, time.Hour, ack.IPAddressLeaseTime(0))

	reboot, err := dhcpv4.NewDiscovery(mac,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(ack.YourIPAddr)),
	)
	require.NoError(t, err)

	ack = conn.exchange(t, reboot)
	require.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
	assert.Less(t, ack.IPAddressLeaseTime(0), time.Hour)
	assert.Greater(t, ack.IPAddressLeaseTime(0), 59*time.Minute)
}

// TestServeFailure checks that the app reports a server that stops serving
// because reading requests fails, but not a server that is stopped.
func TestServeFailure(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		conn := newFakeConn()
		app := startAppWithConn(t, servingDhcpfile, conn)

		conn.readErrs <- errors.New("the interface went away")

		select {
		case err := <-app.Failed():
			assert.ErrorContains(t, err, "stopped serving subnet 127.0.0.0/8")
			assert.ErrorContains(t, err, "the interface went away")
		case <-time.After(5 * time.Second):
			t.Fatal("the failure wasn't reported")
		}
	})

	t.Run("stopped", func(t *testing.T) {
		// reading fails once the server is stopped, but not with
		// net.ErrClosed
		conn := newFakeConn()
		conn.closeErr = errors.New("use of closed file")
		app := startAppWithConn(t, servingDhcpfile, conn)

		// Stop returns once the server stopped serving
		require.NoError(t, app.Stop())

		select {
		case err := <-app.Failed():
			t.Errorf("stopping the server was reported as a failure: %s", err)
		default:
		}
	})
}

// TestStartFailure checks that Start stops the servers it already started if
// a later one fails to start.
func TestStartFailure(t *testing.T) {
	first := newFakeConn()
	calls := 0
	app := provisionApp(t, `
127.0.0.1/8 {
	database memory
}

lo {
	database memory
}
`, func(log.Logger, net.IP, *net.Interface) (net.PacketConn, error) {
		calls++
		if calls == 1 {
			return first, nil
		}

		return nil, assert.AnError
	})

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
