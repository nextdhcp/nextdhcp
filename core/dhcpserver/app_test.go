package dhcpserver_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain makes the servers of all tests find that a device uses each
// address they ask about, unless a test makes them ask a prober of its own,
// so that no test sends ARP requests.
func TestMain(m *testing.M) {
	restore := dhcpserver.SetProbeAddress((&prober{used: true}).probe)
	code := m.Run()
	restore()

	os.Exit(code)
}

// prober tells servers whether a device uses the addresses they ask about.
type prober struct {
	used bool  // whether a device uses each address
	err  error // the error to fail with instead

	mu    sync.Mutex
	asked []string // the addresses asked about, by the server's address
}

func (p *prober) probe(_ context.Context, _ *net.Interface, ip, addr net.IP) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.asked = append(p.asked, fmt.Sprintf("%s by %s", addr, ip))

	return p.used, p.err
}

// questions returns the addresses p was asked about, by the server's
// address.
func (p *prober) questions() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]string(nil), p.asked...)
}

// useProber makes the servers of the test ask p whether a device uses an
// address.
func useProber(t *testing.T, p *prober) {
	t.Helper()

	t.Cleanup(dhcpserver.SetProbeAddress(p.probe))
}

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

// leaseAddress checks that client gets ip with a DISCOVER and a REQUEST.
func (c *fakeConn) leaseAddress(t *testing.T, client net.HardwareAddr, ip string) {
	t.Helper()

	offer, ack := c.bind(t, client)
	assert.Equal(t, dhcpv4.MessageTypeOffer, offer.MessageType())
	assert.Equal(t, ip, offer.YourIPAddr.String())
	assert.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
	assert.Equal(t, ip, ack.YourIPAddr.String())
}

// renew sends a REQUEST of client to renew its lease of ip, and returns the
// server's response.
func (c *fakeConn) renew(t *testing.T, client net.HardwareAddr, ip net.IP) *dhcpv4.DHCPv4 {
	t.Helper()

	renew, err := dhcpv4.New(
		dhcpv4.WithHwAddr(client),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithClientIP(ip),
	)
	require.NoError(t, err)

	return c.exchange(t, renew)
}

// request sends a REQUEST of client for ip, like a client that reboots, and
// returns the server's response.
func (c *fakeConn) request(t *testing.T, client net.HardwareAddr, ip net.IP) *dhcpv4.DHCPv4 {
	t.Helper()

	request, err := dhcpv4.NewDiscovery(client,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(ip)),
	)
	require.NoError(t, err)

	return c.exchange(t, request)
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

		conn.leaseAddress(t, otherClient, "127.0.0.11")
		conn.leaseAddress(t, staticClient, "127.0.0.10")
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

		conn.leaseAddress(t, staticClient, "127.0.0.10")
	})

	t.Run("leased before", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		// another client leases the address before it is assigned to a
		// client
		require.True(t, t.Run("without static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			conn.leaseAddress(t, otherClient, "127.0.0.10")
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
		conn.leaseAddress(t, otherClient, "127.0.0.11")
		conn.leaseAddress(t, staticClient, "127.0.0.10")
	})

	t.Run("not shared", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		// another client leases the address before it is assigned to a
		// client
		require.True(t, t.Run("without static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			conn.leaseAddress(t, otherClient, "127.0.0.10")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, static))

		// the other client may use the address until its lease ends, so the
		// static client gets another one meanwhile
		conn.leaseAddress(t, staticClient, "127.0.0.11")

		// the other client's lease ends when its renewal is refused
		renew, err := dhcpv4.New(
			dhcpv4.WithHwAddr(otherClient),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithClientIP(net.IP{127, 0, 0, 10}),
		)
		require.NoError(t, err)

		nak := conn.exchange(t, renew)
		assert.Equal(t, dhcpv4.MessageTypeNak, nak.MessageType())

		// so the static client's renewal is refused too, and it gets its
		// static address
		renew, err = dhcpv4.New(
			dhcpv4.WithHwAddr(staticClient),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithClientIP(net.IP{127, 0, 0, 11}),
		)
		require.NoError(t, err)

		nak = conn.exchange(t, renew)
		assert.Equal(t, dhcpv4.MessageTypeNak, nak.MessageType())

		conn.leaseAddress(t, staticClient, "127.0.0.10")

		// and the address it had is free again
		conn.leaseAddress(t, otherClient, "127.0.0.11")
	})

	t.Run("static client leased another address before", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("without static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			conn.leaseAddress(t, otherClient, "127.0.0.10")
			conn.leaseAddress(t, staticClient, "127.0.0.11")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, static))

		// the client uses its lease until it tries to renew it, so other
		// clients don't get that address
		conn.leaseAddress(t, thirdClient, "127.0.0.12")
	})

	t.Run("static removed", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("with static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, static))
			conn.leaseAddress(t, staticClient, "127.0.0.10")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))

		// the client keeps its lease until it expires, so other clients don't
		// get the address, and it can renew the lease
		conn.leaseAddress(t, otherClient, "127.0.0.11")

		renew, err := dhcpv4.New(
			dhcpv4.WithHwAddr(staticClient),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithClientIP(net.IP{127, 0, 0, 10}),
		)
		require.NoError(t, err)

		ack := conn.exchange(t, renew)
		assert.Equal(t, dhcpv4.MessageTypeAck, ack.MessageType())
		assert.Equal(t, "127.0.0.10", ack.YourIPAddr.String())
	})

	t.Run("static outside range removed", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("with static", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, "static aa:bb:cc:dd:ee:ff 127.0.0.50"))
			conn.leaseAddress(t, staticClient, "127.0.0.50")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))

		// nothing serves the address anymore, so its renewal is refused
		renew, err := dhcpv4.New(
			dhcpv4.WithHwAddr(staticClient),
			dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
			dhcpv4.WithClientIP(net.IP{127, 0, 0, 50}),
		)
		require.NoError(t, err)

		nak := conn.exchange(t, renew)
		assert.Equal(t, dhcpv4.MessageTypeNak, nak.MessageType())

		// and the client gets an address of the range instead
		conn.leaseAddress(t, staticClient, "127.0.0.10")
	})

	t.Run("failed start", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("lease", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, ""))
			conn.leaseAddress(t, otherClient, "127.0.0.10")
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
		conn.leaseAddress(t, thirdClient, "127.0.0.11")
	})
}

// TestStaticAddressReassigned checks that a static address that is assigned
// to another client is given to it once its previous client tries to renew
// its lease and is refused, without range.
func TestStaticAddressReassigned(t *testing.T) {
	const dhcpfile = "127.0.0.1/8 {\n\tdatabase %s\n\t%s\n}\n"

	client1 := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}
	client2 := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x02}

	t.Run("to another client", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("before", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, "static 00:aa:bb:cc:dd:01 127.0.0.50"))
			conn.leaseAddress(t, client1, "127.0.0.50")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, "static 00:aa:bb:cc:dd:02 127.0.0.50"))

		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, client1, net.IP{127, 0, 0, 50}).MessageType())
		conn.leaseAddress(t, client2, "127.0.0.50")
	})

	t.Run("swapped", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("before", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, "static 00:aa:bb:cc:dd:01 127.0.0.50\n\tstatic 00:aa:bb:cc:dd:02 127.0.0.51"))
			conn.leaseAddress(t, client1, "127.0.0.50")
			conn.leaseAddress(t, client2, "127.0.0.51")
		}))

		conn := startApp(t, fmt.Sprintf(dhcpfile, database, "static 00:aa:bb:cc:dd:01 127.0.0.51\n\tstatic 00:aa:bb:cc:dd:02 127.0.0.50"))

		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, client1, net.IP{127, 0, 0, 50}).MessageType())
		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, client2, net.IP{127, 0, 0, 51}).MessageType())
		conn.leaseAddress(t, client1, "127.0.0.51")
		conn.leaseAddress(t, client2, "127.0.0.50")
	})
}

// TestStaticAddressHandedOver checks that a static address that is still
// leased to another client is given to its client at once if no device on the
// subnet uses it anymore, and only then, also without range.
func TestStaticAddressHandedOver(t *testing.T) {
	const dhcpfile = "127.0.0.1/8 {\n\tdatabase %s\n\t%s\n}\n"

	oldClient := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}
	newClient := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x02}
	static := net.IP{127, 0, 0, 10}

	servers := []struct {
		name          string
		before, after string
	}{
		{
			// the address is assigned to another client
			name:   "static",
			before: "static 00:aa:bb:cc:dd:01 127.0.0.10",
			after:  "static 00:aa:bb:cc:dd:02 127.0.0.10",
		},
		{
			// the address was leased from the range before it was assigned
			name:   "static and range",
			before: "range 127.0.0.10 127.0.0.20",
			after:  "static 00:aa:bb:cc:dd:02 127.0.0.10\n\trange 127.0.0.10 127.0.0.20",
		},
	}

	probes := []struct {
		name       string
		used       bool
		err        error
		handedOver bool
	}{
		{name: "no device", handedOver: true},
		{name: "device", used: true},
		{name: "probe failed", err: assert.AnError},
	}

	for _, server := range servers {
		for _, probe := range probes {
			t.Run(server.name+"/"+probe.name, func(t *testing.T) {
				database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

				require.True(t, t.Run("before", func(t *testing.T) {
					conn := startApp(t, fmt.Sprintf(dhcpfile, database, server.before))
					conn.leaseAddress(t, oldClient, "127.0.0.10")
				}))

				p := &prober{used: probe.used, err: probe.err}
				useProber(t, p)
				conn := startApp(t, fmt.Sprintf(dhcpfile, database, server.after))

				res := conn.request(t, newClient, static)
				assert.Equal(t, []string{"127.0.0.10 by 127.0.0.1"}, p.questions())
				if probe.handedOver {
					assert.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
					assert.Equal(t, "127.0.0.10", res.YourIPAddr.String())
				} else {
					assert.Equal(t, dhcpv4.MessageTypeNak, res.MessageType())
				}

				// the old client's lease, if it's left, ends when it asks
				// for the address
				assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, oldClient, static).MessageType())

				// so the new client has it, or gets it now
				res = conn.request(t, newClient, static)
				assert.Equal(t, dhcpv4.MessageTypeAck, res.MessageType())
				assert.Equal(t, "127.0.0.10", res.YourIPAddr.String())
			})
		}
	}

	t.Run("discover", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("before", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, servers[1].before))
			conn.leaseAddress(t, oldClient, "127.0.0.10")
		}))

		p := &prober{}
		useProber(t, p)
		conn := startApp(t, fmt.Sprintf(dhcpfile, database, servers[1].after))

		conn.leaseAddress(t, newClient, "127.0.0.10")
		assert.Equal(t, []string{"127.0.0.10 by 127.0.0.1"}, p.questions())

		// the old client gets another address
		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, oldClient, static).MessageType())
		conn.leaseAddress(t, oldClient, "127.0.0.11")
	})

	t.Run("renewal of another address", func(t *testing.T) {
		database := "bolt " + filepath.Join(t.TempDir(), "leases.db")

		require.True(t, t.Run("before", func(t *testing.T) {
			conn := startApp(t, fmt.Sprintf(dhcpfile, database, servers[1].before))
			conn.leaseAddress(t, oldClient, "127.0.0.10")
			conn.leaseAddress(t, newClient, "127.0.0.11")
		}))

		p := &prober{}
		useProber(t, p)
		conn := startApp(t, fmt.Sprintf(dhcpfile, database, servers[1].after))

		// the new client is moved to its static address
		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, newClient, net.IP{127, 0, 0, 11}).MessageType())
		assert.Equal(t, []string{"127.0.0.10 by 127.0.0.1"}, p.questions())
		conn.leaseAddress(t, newClient, "127.0.0.10")

		// and the address it had is free again
		assert.Equal(t, dhcpv4.MessageTypeNak, conn.renew(t, oldClient, static).MessageType())
		conn.leaseAddress(t, oldClient, "127.0.0.11")
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

// send sends msg to the server, from the address from, without waiting for
// a response.
func (c *fakeConn) send(t *testing.T, msg *dhcpv4.DHCPv4, from net.IP) {
	t.Helper()

	select {
	case c.requests <- packet{payload: msg.ToBytes(), addr: &net.UDPAddr{IP: from, Port: dhcpv4.ClientPort}}:
	case <-time.After(5 * time.Second):
		t.Fatal("server didn't read the request")
	}
}

// release sends a RELEASE of client for ip, which the server doesn't respond
// to.
func (c *fakeConn) release(t *testing.T, client net.HardwareAddr, ip net.IP) {
	t.Helper()

	release, err := dhcpv4.New(
		dhcpv4.WithHwAddr(client),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRelease),
		dhcpv4.WithClientIP(ip),
	)
	require.NoError(t, err)

	c.send(t, release, ip)
}

// TestReleaseOwnLeaseOnly checks that a client can release its own lease, but
// not the lease of another client, which would let the server lease the
// address to a third client while the other client still uses it.
func TestReleaseOwnLeaseOnly(t *testing.T) {
	owner := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}
	other := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x02}
	ip := net.IP{127, 0, 0, 100}

	for name, tc := range map[string]struct {
		releasedBy net.HardwareAddr
		released   bool
	}{
		"own lease":          {releasedBy: owner, released: true},
		"other client lease": {releasedBy: other, released: false},
		// a client can leave the hardware address out of its requests
		"no hardware address": {releasedBy: net.HardwareAddr{}, released: false},
	} {
		t.Run(name, func(t *testing.T) {
			conn := newFakeConn()
			app := startAppWithConn(t, servingDhcpfile, conn)

			conn.leaseAddress(t, owner, ip.String())
			conn.release(t, tc.releasedBy, ip)

			// Stop returns once the server served all requests it read
			require.NoError(t, app.Stop())

			leases, err := app.Servers[0].Database.Leases(context.Background())
			require.NoError(t, err)

			var holders []string
			for _, l := range leases {
				if l.Address.Equal(ip) {
					holders = append(holders, string(l.ID))
				}
			}

			if tc.released {
				assert.Empty(t, holders)
			} else {
				assert.Equal(t, []string{owner.String()}, holders)
			}
		})
	}
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

// TestNoHardwareAddress checks that clients without hardware address, which
// the lease database identifies clients by, get no address, instead of all of
// them getting the same one.
func TestNoHardwareAddress(t *testing.T) {
	conn := startApp(t, servingDhcpfile)
	ip := net.IP{127, 0, 0, 100}

	for range 2 {
		assert.Equal(t, dhcpv4.MessageTypeNak, conn.request(t, net.HardwareAddr{}, ip).MessageType())
	}

	// the address is still free
	conn.leaseAddress(t, net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}, ip.String())
}

// TestDecline checks that an address that a client declines, as another
// device uses it, isn't leased again, to that client or any other.
func TestDecline(t *testing.T) {
	conn := newFakeConn()
	app := startAppWithConn(t, servingDhcpfile, conn)

	client := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x01}
	other := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0x02}
	declined := net.IP{127, 0, 0, 100}

	conn.leaseAddress(t, client, declined.String())

	decline, err := dhcpv4.New(
		dhcpv4.WithHwAddr(client),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(declined)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.IP{127, 0, 0, 1})),
	)
	require.NoError(t, err)
	conn.send(t, decline, net.IPv4zero)

	// the server doesn't respond, so wait until the database holds that the
	// address is reserved, but not for the client
	require.Eventually(t, func() bool {
		reserved, err := app.Servers[0].Database.ReservedAddresses(context.Background())
		r := reserved.FindIP(declined)

		return err == nil && r != nil && r.ID != lease.HardwareAddrID(client)
	}, 5*time.Second, 10*time.Millisecond)

	conn.leaseAddress(t, client, "127.0.0.101")
	conn.leaseAddress(t, other, "127.0.0.102")
}
