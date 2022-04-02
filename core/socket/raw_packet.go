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

	err := udp.SetNetworkLayerForChecksum(ip)

	if err != nil {
		return nil, err
	}

	err = gopacket.SerializeLayers(buf, opts,
		ethernet,
		ip,
		udp,
		gopacket.Payload(payload))

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func extractUDPPayloads(targetPort int, b []byte) ([]byte, net.Addr, bool) {
	packet := gopacket.NewPacket(b, layers.LayerTypeEthernet, gopacket.Default)
	if err := packet.ErrorLayer(); err != nil {
		return nil, nil, false
	}

	phy, ok := packet.LinkLayer().(*layers.Ethernet)
	if !ok {
		return nil, nil, false
	}

	ipLayer, ok := packet.NetworkLayer().(*layers.IPv4)
	if !ok {
		return nil, nil, false
	}

	udpLayer, ok := packet.TransportLayer().(*layers.UDP)
	if !ok {
		return nil, nil, false
	}

	if uint16(udpLayer.DstPort) != uint16(targetPort) {
		return nil, nil, false
	}

	if len(udpLayer.Payload) == 0 {
		return nil, nil, false
	}

	return udpLayer.Payload, &Addr{
		RawAddr: RawAddr{
			MAC:  phy.SrcMAC,
			IP:   ipLayer.SrcIP.To4(),
			Port: uint16(udpLayer.SrcPort),
		},
		Local: RawAddr{
			MAC:  phy.DstMAC,
			IP:   ipLayer.DstIP.To4(),
			Port: uint16(udpLayer.DstPort),
		},
	}, true
}
