package server

import (
	"errors"
	"log"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/mdlayher/raw"
)

type Listener interface {
	net.PacketConn

	SendRaw(dstIP net.IP, dstMac net.HardwareAddr, payload []byte) error

	Raw() net.PacketConn

	IP() net.IP
}

type listener struct {
	// The embedded net.PacketConn is a net.UDPConn that
	// is used to receive DHCP requests and
	// send routed UDP packets to clients in RENEWING state
	// as well as DHCP relay agents (once supported ...).
	net.PacketConn

	// rawConn is used to send directed unicasts without prior ARP requests.
	// All packets received on this connection are read and discarded immediately
	// as they should be duplicates (already received on rawConn)
	rawConn net.PacketConn

	// iface is the interface we are listening on
	iface net.Interface

	// IP is the IP address we are listening on
	ip net.IP
}

func NewListener(ip net.IP) (Listener, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	found := false
	var iface net.Interface
L:
	for _, iface = range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.Equal(ip) {
				found = true
				break L
			}
		}
	}

	if !found {
		return nil, errors.New("failed to locate network interface")
	}

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.IPv4zero, // TODO
		Port: dhcpv4.ServerPort,
	})
	if err != nil {
		return nil, err
	}

	rawConn, err := raw.ListenPacket(&iface, syscall.ETH_P_ALL, &raw.Config{LinuxSockDGRAM: false})
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	c := &listener{
		rawConn:    rawConn,
		PacketConn: udpConn,
		iface:      iface,
		ip:         ip,
	}

	go c.discardRAWInput()

	log.Printf("Opened sockets on %s with address %s", iface.Name, ip.String())

	return c, nil
}

func (c *listener) IP() net.IP {
	return c.ip
}

func (c *listener) SendRaw(dstIP net.IP, dstMAC net.HardwareAddr, payload []byte) error {
	buf := gopacket.NewSerializeBuffer()

	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	ethernet := &layers.Ethernet{
		DstMAC:       dstMAC,
		SrcMAC:       c.iface.HardwareAddr,
		EthernetType: layers.EthernetTypeIPv4,
	}

	ip := &layers.IPv4{
		Version:  4,
		TTL:      255,
		SrcIP:    c.ip,
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
		return err
	}

	data := buf.Bytes()
	_, err = c.rawConn.WriteTo(data, &raw.Addr{
		HardwareAddr: dstMAC,
	})

	return err
}

// Close closes both connections and returns the first error encountered
func (c *listener) Close() error {
	err := c.PacketConn.Close()

	if e := c.rawConn.Close(); e != nil && err == nil {
		err = e
	}

	return err
}

func (c *listener) Raw() net.PacketConn {
	return c.rawConn
}

func (c *listener) discardRAWInput() {
	b := make([]byte, 1024)
	for {
		_, _, err := c.rawConn.ReadFrom(b)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Timeout() || opErr.Temporary() {
					continue
				}
			}

			return
		}
		//log.Printf("discarded raw packet ...")
	}
}
