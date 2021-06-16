package socket

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/mdlayher/raw"
	"github.com/nextdhcp/nextdhcp/core/log"
	interfaces "github.com/nextdhcp/nextdhcp/core/utils/iface"
)

var (
	rawListenPacket = func(iface *net.Interface) (net.PacketConn, error) {
		// TODO(ppacher): use the BPF filter support to drop not-DHCP related
		// packets
		return raw.ListenPacket(iface, 0x800, nil)
	}

	udpListenPacket = func(ip net.IP, port int) (net.PacketConn, error) {
		return net.ListenUDP("udp4", &net.UDPAddr{
			IP:   ip,
			Port: port,
		})
	}
)

// ListenDHCP starts listening for DHCP requests on the given IP and interface
// It opens a UDP and a AF_PACKET socket for communication
func ListenDHCP(l log.Logger, ip net.IP, iface *net.Interface) (net.PacketConn, error) {
	// If not interface is provided try to lookup the correct one
	if iface == nil {
		var err error
		iface, err = interfaces.ByIP(ip)
		if err != nil {
			return nil, err
		}
	}

	udp, err := udpListenPacket(ip, dhcpv4.ServerPort)
	if err != nil {
		return nil, err
	}

	r, err := rawListenPacket(iface)
	if err != nil {
		return nil, err
	}

	p := &DHCPConn{
		udp:   udp,
		raw:   r,
		iface: iface,
		ip:    ip,
		l:     l,
	}

	p.wg.Add(1)
	go p.discardUDP()

	return p, nil
}

// DHCPConn implements net.PacketConn but utilizes a standard UDP and
// and AF_PACKET socket
type DHCPConn struct {
	udp   net.PacketConn // used for routable unicasts
	raw   net.PacketConn // used for directed (w/o ARP) unicasts
	iface *net.Interface // the interface the raw PacketConn is bound to
	ip    net.IP         // the listening IP for the udp PacketConn
	wg    sync.WaitGroup
	l     log.Logger
}

// Close will close both the UDP and the AF_PACKET socket and
// will return the first error encountered
func (p *DHCPConn) Close() error {
	firstErr := p.udp.Close()

	secondErr := p.raw.Close()
	if secondErr != nil && firstErr == nil {
		firstErr = secondErr
	}

	// wait for discardUDP to finish
	p.wg.Wait()

	return firstErr
}

// LocalAddr implements the PacketConn interface and returns
// the local address of the UDP socket
func (p *DHCPConn) LocalAddr() net.Addr {
	return p.udp.LocalAddr()
}

// ReadFrom implements the PacketConn interface and calls
// ReadFrom on the underlying AF_PACKET socket
func (p *DHCPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	buf := make([]byte, 4096)
	for {
		n, _, err := p.raw.ReadFrom(buf)

		if n > 0 {
			payload, addr, ok := extractUDPPayloads(dhcpv4.ServerPort, buf[:n])
			if ok {
				// TODO(ppacher): the following check does not adhere to
				// the expected ReadFrom behavior. Fix it
				if cap(b) < len(payload) {
					return 0, nil, fmt.Errorf("buffer size to small")
				}

				// copy over the payload
				for i, x := range payload {
					b[i] = x
				}

				return len(payload), addr, err
			}
		}

		if err != nil {
			return 0, nil, err
		}
	}
}

// WriteTo sends a packet to the given addr. If addr is a *Addr the AF_PACKET
// socket will be choosen. Otherwise, for e.g. net.UDPAddr, the underlying UDP
// packet conn will be used.
// It implements the PacketConn interface
func (p *DHCPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if r, ok := addr.(*Addr); ok {
		srcMAC := p.iface.HardwareAddr
		srcIP := p.ip

		if r.Local.MAC != nil {
			srcMAC = r.Local.MAC
		}

		if r.Local.IP != nil {
			srcIP = r.Local.IP
		}

		p.l.Debugf("[socket] sending directed (raw) unicast %s (%s) -> %s (%s)", srcIP, srcMAC, r.IP, r.MAC)

		// FIXME(ppacher): Ports are currenty hardcoded in PreparePacket
		payload, err := PreparePacket(srcMAC, srcIP, r.MAC, r.IP, b)
		if err != nil {
			return 0, err
		}

		return p.raw.WriteTo(payload, &raw.Addr{
			HardwareAddr: r.MAC,
		})
	}

	p.l.Debugf("[socket] sending (routed) UDP response %s -> %s", p.udp.LocalAddr(), addr)
	return p.udp.WriteTo(b, addr)
}

// SetDeadline implements the PacketConn interface
// but is not yet implemented
func (p *DHCPConn) SetDeadline(t time.Time) error {
	// TODO(ppacher): can we use p.udp.SetDeadline and p.raw.SetDeadline instead?
	// If, we need to check for any deadline errors in discardUDP()
	firstErr := p.SetReadDeadline(t)
	if secondErr := p.SetWriteDeadline(t); secondErr != nil && firstErr == nil {
		firstErr = secondErr
	}

	return firstErr
}

// SetReadDeadline implements the PacketConn interface
func (p *DHCPConn) SetReadDeadline(t time.Time) error {
	return p.raw.SetReadDeadline(t)
}

// SetWriteDeadline implements the PacketConn interface
func (p *DHCPConn) SetWriteDeadline(t time.Time) error {
	firstErr := p.raw.SetWriteDeadline(t)
	if secondErr := p.udp.SetWriteDeadline(t); secondErr != nil && firstErr == nil {
		firstErr = secondErr
	}

	return firstErr
}

func (p *DHCPConn) discardUDP() {
	buf := make([]byte, 1024)

	for {
		_, _, err := p.udp.ReadFrom(buf)

		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Timeout() || opErr.Temporary() {
					continue
				}
			}

			return
		}
	}
}

var _ net.PacketConn = &DHCPConn{}
