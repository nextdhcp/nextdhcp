package socket

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/log"
	interfaces "github.com/nextdhcp/nextdhcp/core/utils/iface"
	"golang.org/x/net/ipv4"
)

// udpSocket is the part of an *ipv4.PacketConn that the connections of
// ListenUDP share. Tests replace it so they don't need port 67.
type udpSocket interface {
	ReadFrom(b []byte) (int, *ipv4.ControlMessage, net.Addr, error)
	WriteTo(b []byte, cm *ipv4.ControlMessage, dst net.Addr) (int, error)
	LocalAddr() net.Addr
	Close() error
}

const (
	// udpQueueLen is how many requests a connection of ListenUDP holds for
	// its server. The listener drops the requests that arrive while it's
	// full.
	udpQueueLen = 256

	// maxUDPPayload is the size of the largest UDP payload.
	maxUDPPayload = 1<<16 - 1
)

var (
	// udpListenAddr is the address of the socket that the connections of
	// ListenUDP share: the DHCP server port of all addresses, because only a
	// UDP socket that isn't bound to an address receives the broadcasts of
	// clients that don't have one yet. Tests replace it so they don't need
	// port 67.
	udpListenAddr = fmt.Sprintf(":%d", dhcpv4.ServerPort)

	// udpListen opens the socket that the connections of ListenUDP share.
	// Tests replace it.
	udpListen = listenUDPSocket

	// listInterfaces lists the network interfaces. Tests replace it.
	listInterfaces = net.Interfaces

	// interfaceByIP returns the network interface that has the address ip.
	// Tests replace it so they can serve addresses the host doesn't have.
	interfaceByIP = interfaces.ByIP

	// interfaceCheckInterval is how often a UDP listener checks that the
	// interfaces of its connections still exist.
	interfaceCheckInterval = 5 * time.Second
)

var (
	// udpMu guards udpShared and the connections of each udpListener.
	udpMu sync.Mutex

	// udpShared is the listener that new connections of ListenUDP join, or
	// nil if no connection is open.
	udpShared *udpListener
)

// ListenUDP returns a connection for the DHCP server with the address ip on
// iface that, unlike the connections of ListenDHCP, uses no AF_PACKET socket,
// so it doesn't need the CAP_NET_RAW capability.
//
// The connections of all servers share one UDP socket on the DHCP server port
// of all addresses, because only a socket that isn't bound to an address
// receives the broadcasts of clients that don't have one yet. Each connection
// reads the requests that arrive on its server's interface, except that a
// request sent to the address of another server on the interface, like a
// renewal, only goes to that server.
//
// A connection sends replies from ip, to *net.UDPAddr addresses. It can't
// send to the hardware address of a client that doesn't have an IP address
// yet, so servers broadcast the replies that they send that way through the
// connections of ListenDHCP, which RFC 2131 allows. Reading from a connection
// fails once its interface goes away.
//
// Like binding a socket to ip, ListenUDP fails if ip isn't an address of this
// host, or if another connection already serves ip.
func ListenUDP(l log.Logger, ip net.IP, iface *net.Interface) (net.PacketConn, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("the UDP transport serves IPv4 addresses, not %s", ip)
	}

	// The shared socket isn't bound to ip, so check, as binding a socket to
	// ip would, that ip is an address of this host. It may be assigned to
	// another interface than iface.
	ifc, err := interfaceByIP(ip4)
	if err != nil {
		return nil, fmt.Errorf("%s is not an address of this host: %w", ip4, err)
	}

	// If no interface is provided, use the one that has ip
	if iface == nil {
		iface = ifc
	}

	udpMu.Lock()
	defer udpMu.Unlock()

	if udpShared == nil {
		sock, err := udpListen(udpListenAddr)
		if err != nil {
			return nil, err
		}

		udpShared = newUDPListener(sock)
	}

	// As binding a second socket to ip would, refuse a second connection for
	// ip: its server would answer the requests of the other one
	for other := range udpShared.conns {
		if other.ip.Equal(ip4) {
			return nil, fmt.Errorf("another server already serves %s: %w", ip4, syscall.EADDRINUSE)
		}
	}

	c := &udpConn{
		listener:      udpShared,
		ip:            ip4,
		iface:         *iface,
		l:             l,
		in:            make(chan udpPacket, udpQueueLen),
		closed:        make(chan struct{}),
		failed:        make(chan struct{}),
		readDeadline:  makeDeadline(),
		writeDeadline: makeDeadline(),
	}
	udpShared.conns[c] = struct{}{}

	l.Debugf("[socket] serving %s (%s) through the shared UDP socket %s", ip4, iface.Name, udpShared.sock.LocalAddr())

	return c, nil
}

// listenUDPSocket opens a UDP socket on addr that reports the interface each
// datagram arrived on and the address it was sent to.
func listenUDPSocket(addr string) (udpSocket, error) {
	c, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, err
	}

	p := ipv4.NewPacketConn(c)
	if err := p.SetControlMessage(ipv4.FlagInterface|ipv4.FlagDst, true); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("failed to ask for the interface and destination of requests on %s: %w", addr, err)
	}

	return p, nil
}

// udpListener is the socket that the connections of ListenUDP share. It
// reads the requests for all of them and passes each one on to the
// connections it is for.
type udpListener struct {
	sock  udpSocket
	conns map[*udpConn]struct{} // guarded by udpMu

	stopOnce sync.Once
	stopped  chan struct{} // closed once the socket is closed
}

func newUDPListener(sock udpSocket) *udpListener {
	l := &udpListener{
		sock:    sock,
		conns:   make(map[*udpConn]struct{}),
		stopped: make(chan struct{}),
	}

	go l.read()
	go l.watchInterfaces(listInterfaces, interfaceCheckInterval)

	return l
}

// read reads requests from the socket until reading fails, for example
// because the socket was closed.
func (l *udpListener) read() {
	buf := make([]byte, maxUDPPayload)

	for {
		n, cm, src, err := l.sock.ReadFrom(buf)
		if err != nil {
			l.fail(err)
			return
		}

		// Without the interface a request arrived on, there is no telling
		// which server it is for
		if n > 0 && cm != nil {
			l.dispatch(buf[:n], cm, src)
		}
	}
}

// dispatch passes a request, which arrived on the interface with the index
// cm.IfIndex, to the connections of the servers on that interface. A request
// sent to the address of one of them only goes to that one.
func (l *udpListener) dispatch(payload []byte, cm *ipv4.ControlMessage, src net.Addr) {
	udpMu.Lock()
	var to []*udpConn
	for c := range l.conns {
		if c.iface.Index != cm.IfIndex {
			continue
		}

		if c.ip.Equal(cm.Dst) {
			to = []*udpConn{c}
			break
		}

		to = append(to, c)
	}
	udpMu.Unlock()

	if len(to) == 0 {
		return
	}

	// The connections copy the payload into the buffers they read into, so
	// they can share one copy
	payload = append([]byte(nil), payload...)
	for _, c := range to {
		c.deliver(udpPacket{payload: payload, src: copyAddr(src)})
	}
}

// watchInterfaces makes reading from connections fail once their interface
// goes away, as reading from the connections of ListenDHCP does. Every
// interval until the socket is closed, it looks for their interfaces in the
// interfaces that list returns.
func (l *udpListener) watchInterfaces(list func() ([]net.Interface, error), interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-l.stopped:
			return
		case <-t.C:
		}

		exist, err := interfaceIndexes(list)
		if err != nil {
			// try again next time
			continue
		}

		// The kernel doesn't list the interfaces atomically: before Linux
		// 6.9, a listing can skip one that exists while others come and go.
		// So only give up on the interfaces that a second listing lacks too.
		if l.lacksInterface(exist) {
			again, err := interfaceIndexes(list)
			if err != nil {
				continue
			}

			for index := range again {
				exist[index] = true
			}
		}

		udpMu.Lock()
		for c := range l.conns {
			if !exist[c.iface.Index] {
				c.fail(fmt.Errorf("network interface %s went away", c.iface.Name))
			}
		}
		udpMu.Unlock()
	}
}

// interfaceIndexes returns the indexes of the network interfaces that list
// returns.
func interfaceIndexes(list func() ([]net.Interface, error)) (map[int]bool, error) {
	ifaces, err := list()
	if err != nil {
		return nil, err
	}

	exist := make(map[int]bool, len(ifaces))
	for _, ifc := range ifaces {
		exist[ifc.Index] = true
	}

	return exist, nil
}

// lacksInterface reports whether the interface of a connection isn't one of
// the interfaces with the indexes in exist.
func (l *udpListener) lacksInterface(exist map[int]bool) bool {
	udpMu.Lock()
	defer udpMu.Unlock()

	for c := range l.conns {
		if !exist[c.iface.Index] {
			return true
		}
	}

	return false
}

// fail makes reading from all connections fail with err, which reading from
// the socket failed with, and closes the socket, unless it failed because it
// was closed.
func (l *udpListener) fail(err error) {
	udpMu.Lock()
	defer udpMu.Unlock()

	select {
	case <-l.stopped:
		return
	default:
	}

	for c := range l.conns {
		c.fail(fmt.Errorf("reading from the shared UDP socket failed: %w", err))
	}

	_ = l.stopLocked()
}

// remove removes c from the listener, and closes the socket once no
// connection is left.
func (l *udpListener) remove(c *udpConn) error {
	udpMu.Lock()
	defer udpMu.Unlock()

	delete(l.conns, c)
	if len(l.conns) > 0 {
		return nil
	}

	return l.stopLocked()
}

// stopLocked closes the socket, if it isn't closed yet, so that new
// connections of ListenUDP open a new one. udpMu must be held.
func (l *udpListener) stopLocked() error {
	if udpShared == l {
		udpShared = nil
	}

	var err error
	l.stopOnce.Do(func() {
		close(l.stopped)
		err = l.sock.Close()
	})

	return err
}

// udpPacket is a request for the server of a udpConn.
type udpPacket struct {
	payload []byte
	src     net.Addr
}

// udpConn is the connection of one server to the socket that the
// connections of ListenUDP share. It implements net.PacketConn.
type udpConn struct {
	listener *udpListener
	ip       net.IP        // the server's address, which replies come from
	iface    net.Interface // the interface the server serves
	l        log.Logger

	in chan udpPacket // the requests for the server

	closeOnce sync.Once
	closed    chan struct{} // closed by Close

	failOnce sync.Once
	failed   chan struct{} // closed once the connection failed
	failErr  error         // why it failed, set before failed is closed

	readDeadline  connDeadline
	writeDeadline connDeadline
}

// deliver passes p on to the server, unless too many requests are waiting
// for it already.
func (c *udpConn) deliver(p udpPacket) {
	select {
	case c.in <- p:
	default:
		c.l.Debugf("[socket] dropping a request from %s: %d requests are waiting to be served", p.src, udpQueueLen)
	}
}

// fail makes reading from c fail with err, unless it already failed.
func (c *udpConn) fail(err error) {
	c.failOnce.Do(func() {
		c.failErr = err
		close(c.failed)
	})
}

// stopErr returns net.ErrClosed if c was closed, or why it failed.
func (c *udpConn) stopErr() error {
	select {
	case <-c.closed:
		return net.ErrClosed
	default:
	}

	select {
	case <-c.failed:
		return c.failErr
	default:
	}

	return nil
}

// ReadFrom reads the next request for the server. It implements
// net.PacketConn, and returns the address the request came from as a
// *net.UDPAddr.
func (c *udpConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if err := c.stopErr(); err != nil {
		return 0, nil, c.opError("read", nil, err)
	}

	select {
	case <-c.readDeadline.wait():
		return 0, nil, c.opError("read", nil, os.ErrDeadlineExceeded)
	default:
	}

	select {
	case p := <-c.in:
		// Like a UDP socket, copy as much of the payload as fits
		return copy(b, p.payload), p.src, nil
	case <-c.closed:
		return 0, nil, c.opError("read", nil, net.ErrClosed)
	case <-c.failed:
		return 0, nil, c.opError("read", nil, c.failErr)
	case <-c.readDeadline.wait():
		return 0, nil, c.opError("read", nil, os.ErrDeadlineExceeded)
	}
}

// WriteTo sends b from the server's address to addr, which must be a
// *net.UDPAddr. It implements net.PacketConn.
func (c *udpConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if err := c.stopErr(); err != nil {
		return 0, c.opError("write", addr, err)
	}

	if isClosed(c.writeDeadline.wait()) {
		return 0, c.opError("write", addr, os.ErrDeadlineExceeded)
	}

	dst, ok := addr.(*net.UDPAddr)
	if !ok || dst.IP.To4() == nil {
		return 0, c.opError("write", addr, net.InvalidAddrError("not an IPv4 UDP address"))
	}

	// The socket isn't bound to the server's address, so set the source of
	// the reply. Send replies to clients, on the client port, through the
	// server's interface, like the connections of ListenDHCP do: the kernel
	// doesn't route a limited broadcast by its destination, and the server's
	// address may be on another interface, whose routes don't lead to the
	// clients. Replies to relay agents, on the server port, are routed.
	cm := &ipv4.ControlMessage{Src: c.ip}
	if dst.IP.Equal(net.IPv4bcast) || dst.Port == dhcpv4.ClientPort {
		cm.IfIndex = c.iface.Index
	}

	c.l.Debugf("[socket] sending UDP response %s -> %s (%s)", c.ip, dst, c.iface.Name)

	n, err := c.listener.sock.WriteTo(b, cm, dst)
	if err != nil {
		// Name the server's address, which the reply is sent from, instead of
		// the shared socket's
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			err = opErr.Err
		}

		return n, c.opError("write", dst, err)
	}

	return n, nil
}

// Close closes c. It closes the shared socket if no other connection uses
// it anymore. It implements net.PacketConn.
func (c *udpConn) Close() error {
	err := c.opError("close", nil, net.ErrClosed)
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.listener.remove(c)
	})

	return err
}

// LocalAddr returns the server's address, with the port of the shared
// socket. It implements net.PacketConn.
func (c *udpConn) LocalAddr() net.Addr {
	port := dhcpv4.ServerPort
	if a, ok := c.listener.sock.LocalAddr().(*net.UDPAddr); ok {
		port = a.Port
	}

	return &net.UDPAddr{IP: c.ip, Port: port}
}

// SetDeadline implements net.PacketConn.
func (c *udpConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}

	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.PacketConn.
func (c *udpConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.set(t)
	return nil
}

// SetWriteDeadline implements net.PacketConn. Writing to a UDP socket
// doesn't wait, so WriteTo only checks whether the deadline has passed.
func (c *udpConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.set(t)
	return nil
}

func (c *udpConn) opError(op string, addr net.Addr, err error) error {
	return &net.OpError{Op: op, Net: "udp4", Source: c.LocalAddr(), Addr: addr, Err: err}
}

// copyAddr returns a copy of addr if it's a *net.UDPAddr, so that servers
// don't share it, or addr itself.
func copyAddr(addr net.Addr) net.Addr {
	a, ok := addr.(*net.UDPAddr)
	if !ok {
		return addr
	}

	cp := *a
	cp.IP = append(net.IP(nil), a.IP...)

	return &cp
}

// connDeadline is a deadline that goroutines can wait for, like those of the
// connections of net.Pipe.
type connDeadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	passed chan struct{} // closed once the deadline has passed
}

func makeDeadline() connDeadline {
	return connDeadline{passed: make(chan struct{})}
}

// set sets the deadline to t, or removes it if t is the zero time.
func (d *connDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If the timer of the old deadline already fired, wait until it closed
	// passed
	if d.timer != nil && !d.timer.Stop() {
		<-d.passed
	}
	d.timer = nil

	passed := isClosed(d.passed)
	if t.IsZero() {
		if passed {
			d.passed = make(chan struct{})
		}

		return
	}

	if dur := time.Until(t); dur > 0 {
		if passed {
			d.passed = make(chan struct{})
		}

		ch := d.passed
		d.timer = time.AfterFunc(dur, func() { close(ch) })

		return
	}

	if !passed {
		close(d.passed)
	}
}

// wait returns a channel that is closed once the deadline has passed.
func (d *connDeadline) wait() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.passed
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

var _ net.PacketConn = (*udpConn)(nil)
