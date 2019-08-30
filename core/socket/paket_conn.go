package socket

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/mdlayher/raw"
)

var (
	rawListenPacket = func(iface *net.Interface) (net.PacketConn, error) {
		// TODO(ppacher): use the BPF filter support to drop not-DHCP related
		// packets
		return raw.ListenPacket(iface, syscall.ETH_P_IP, nil)
	}

	udpListenPacket = func(ip net.IP, port int) (net.PacketConn, error) {
		return net.ListenUDP("udp4", &net.UDPAddr{
			IP:   ip,
			Port: port,
		})
	}
)

func interfaceByIP(ip net.IP) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, ifn := range ifaces {
		addrs, err := ifn.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				// Not a IP network so we can safely skip it
				continue
			}

			if n.Contains(ip) {
				return &ifn, nil
			}
		}
	}

	return nil, fmt.Errorf("interface not found for IP %s", ip.String())
}

// ListenDHCP starts listening for DHCP requests on the given IP and interface
// It opens a UDP and a AF_PACKET socket for communication
func ListenDHCP(ip net.IP, iface *net.Interface) (net.PacketConn, error) {
	// If not interface is provided try to lookup the correct one
	if iface == nil {
		var err error
		iface, err = interfaceByIP(ip)
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

	return &packetConn{
		udp:   udp,
		raw:   r,
		iface: iface,
		ip:    ip,
	}, nil
}

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
