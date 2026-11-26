package socket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mdlayher/packet"
)

// ethernetBroadcast is the hardware address of all hosts on an Ethernet
// network.
var ethernetBroadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

var (
	// arpListenPacket opens the socket that ProbeARP sends ARP requests
	// through and reads ARP packets from. Tests replace it so they don't need
	// raw sockets.
	arpListenPacket = func(iface *net.Interface) (net.PacketConn, error) {
		return packet.Listen(iface, packet.Raw, int(layers.EthernetTypeARP), nil)
	}

	// arpRequests is how many ARP requests ProbeARP sends, arpInterval
	// apart, so that a lost request or reply doesn't make it miss a device.
	// It decides that no device uses the address an arpInterval after the
	// last request.
	arpRequests = 3
	arpInterval = time.Second / 3
)

// ProbeARP reports whether a device on the network of iface uses ip. It asks
// for the hardware address of ip with ARP requests, like hosts do before they
// send to an address on their network, from srcIP, the address of iface on
// the network. A device that answers, or that sends other ARP packets from
// ip, uses it. If none does within about a second, ProbeARP reports that no
// device uses ip, so a device that doesn't answer ARP requests, for example
// while it's asleep, counts as not using it. ARP only reaches devices on the
// same network segment as iface.
//
// ProbeARP fails, instead of reporting that no device uses ip, if it can't
// send the requests or read the answers, or if ctx is done before it
// decides.
func ProbeARP(ctx context.Context, iface *net.Interface, srcIP, ip net.IP) (bool, error) {
	request, err := arpRequest(iface.HardwareAddr, srcIP, ip)
	if err != nil {
		return false, err
	}

	conn, err := arpListenPacket(iface)
	if err != nil {
		return false, fmt.Errorf("failed to open ARP socket on %s: %w", iface.Name, err)
	}
	defer conn.Close()

	buf := make([]byte, 1500)
	for range arpRequests {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		if _, err := conn.WriteTo(request, &packet.Addr{HardwareAddr: ethernetBroadcast}); err != nil {
			return false, fmt.Errorf("failed to send ARP request for %s: %w", ip, err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(arpInterval)); err != nil {
			return false, err
		}

		for {
			// The socket also reads the requests sent from iface, ours
			// included, and ARP packets of other addresses, whose sender
			// address differs. Packets that iface sends from ip itself show
			// that this host uses it.
			n, _, err := conn.ReadFrom(buf)
			if sender, ok := arpSender(buf[:n]); ok && sender.Equal(ip) {
				return true, nil
			}

			if errors.Is(err, os.ErrDeadlineExceeded) {
				break
			}

			if err != nil {
				return false, fmt.Errorf("failed to read ARP packets on %s: %w", iface.Name, err)
			}
		}
	}

	return false, ctx.Err()
}

// arpRequest returns an Ethernet frame for all hosts with an ARP request for
// the hardware address of ip, from the host with the hardware address srcMAC
// and the IPv4 address srcIP.
func arpRequest(srcMAC net.HardwareAddr, srcIP, ip net.IP) ([]byte, error) {
	// The ARP layer takes the length of each address from its slice, so it
	// needs the 4-byte form of the IPv4 addresses
	src4, ip4 := srcIP.To4(), ip.To4()
	switch {
	case len(srcMAC) != 6:
		return nil, fmt.Errorf("ARP requests need an Ethernet address, not %q", srcMAC.String())
	case src4 == nil:
		return nil, fmt.Errorf("ARP requests need an IPv4 address to send from, not %s", srcIP)
	case ip4 == nil:
		return nil, fmt.Errorf("ARP requests are for IPv4 addresses, not %s", ip)
	}

	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true},
		&layers.Ethernet{
			SrcMAC:       srcMAC,
			DstMAC:       ethernetBroadcast,
			EthernetType: layers.EthernetTypeARP,
		},
		&layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			Operation:         layers.ARPRequest,
			SourceHwAddress:   srcMAC,
			SourceProtAddress: src4,
			DstHwAddress:      make([]byte, 6),
			DstProtAddress:    ip4,
		},
	)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// arpSender returns the IPv4 address that sent frame, if it's an Ethernet
// frame with an ARP packet for IPv4 addresses.
func arpSender(frame []byte) (net.IP, bool) {
	p := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)

	arp, ok := p.Layer(layers.LayerTypeARP).(*layers.ARP)
	if !ok || arp.Protocol != layers.EthernetTypeIPv4 || len(arp.SourceProtAddress) != net.IPv4len {
		return nil, false
	}

	return net.IP(arp.SourceProtAddress), true
}
