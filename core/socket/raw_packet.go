package socket

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// PreparePacket prepares a raw UDP network packet including Ethernet, IP and UDP layers
func PreparePacket(srcMAC net.HardwareAddr, srcIP net.IP, dstMAC net.HardwareAddr, dstIP net.IP, payload []byte) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()

	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	ethernet := &layers.Ethernet{
		DstMAC:       dstMAC,
		SrcMAC:       srcMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}

	ip := &layers.IPv4{
		Version:  4,
		TTL:      255,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Protocol: layers.IPProtocolUDP,
		Flags:    layers.IPv4DontFragment,
	}

	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}

	udp.SetNetworkLayerForChecksum(ip)

	err := gopacket.SerializeLayers(buf, opts,
		ethernet,
		ip,
		udp,
		gopacket.Payload(payload))

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
