package socket

import (
	"errors"
	"net"
	"time"

	"github.com/mdlayher/raw"
)

// PacketConn implements net.PacketConn but utilizes a standard UDP and
// and AF_PACKET socket
type packetConn struct {
	udp   net.PacketConn // used for routable unicasts
	raw   net.PacketConn // used for directed (w/o ARP) unicasts
	iface *net.Interface // the interface the raw PacketConn is bound to
	ip    net.IP         // the listening IP for the udp PacketConn
}

// Close will close both the UDP and the AF_PACKET socket and
// will return the first error encountered
func (p *packetConn) Close() error {
	firstErr := p.udp.Close()

	secondErr := p.raw.Close()
	if secondErr != nil && firstErr == nil {
		firstErr = secondErr
	}

	return firstErr
}

// LocalAddr implements the PacketConn interface and returns
// the local address of the UDP socket
func (p *packetConn) LocalAddr() net.Addr {
	return p.udp.LocalAddr()
}

// ReadFrom implements the PacketConn interface and calls
// ReadFrom on the underlying AF_PACKET socket
func (p *packetConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// TODO(ppacher): only return DHCP requests arriving for the UDP/RAw socket
	return p.raw.ReadFrom(b)
}

// WriteTo sends a packet to the given addr. If addr is a *Addr the AF_PACKET
// socket will be choosen. Otherwise, for e.g. net.UDPAddr, the underlying UDP
// packet conn will be used.
// It implements the PacketConn interface
func (p *packetConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if r, ok := addr.(*Addr); ok {
		srcMAC := p.iface.HardwareAddr
		srcIP := p.ip

		if r.Source.MAC != nil {
			srcMAC = r.Source.MAC
		}

		if r.Source.IP != nil {
			srcIP = r.Source.IP
		}

		// FIXME(ppacher): Ports are currenty hardcoded in PreparePacket
		payload, err := PreparePacket(srcMAC, srcIP, r.MAC, r.IP, b)
		if err != nil {
			return 0, err
		}

		return p.raw.WriteTo(payload, &raw.Addr{
			HardwareAddr: r.MAC,
		})
	}

	return p.udp.WriteTo(b, addr)
}

// SetDeadline implements the PacketConn interface
// but is not yet implemented
func (p *packetConn) SetDeadline(t time.Time) error {
	return errors.New("SetDeadline: not implemented")
}

// SetReadDeadline implements the PacketConn interface
// but is not yet implemented
func (p *packetConn) SetReadDeadline(t time.Time) error {
	return errors.New("SetReadDeadline: not implemented")
}

// SetWriteDeadline implements the PacketConn interface
// but is not yet implemented
func (p *packetConn) SetWriteDeadline(t time.Time) error {
	return errors.New("SetWriteDeadline: not implemented")
}

var _ net.PacketConn = &packetConn{}
