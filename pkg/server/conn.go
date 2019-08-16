package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/mdlayher/raw"
)

type Conn interface {
	io.Closer

	SendRaw(dstIP net.IP, dstMac net.HardwareAddr, payload []byte) error
	Recv(context.Context) (*Request, error)

	Raw() net.PacketConn
	UDP() net.PacketConn

	IP() net.IP
}

type Request struct {
	Peer       *net.UDPAddr
	PeerHwAddr net.HardwareAddr
	Message    *dhcpv4.DHCPv4
	Iface      net.Interface
}

type listener struct {
	udpConn net.PacketConn

	rawConn net.PacketConn

	requests chan *Request

	// iface is the interface we are listening on
	iface net.Interface

	// IP is the IP address we are listening on
	ip net.IP
}

func NewConn(ip net.IP) (Conn, error) {
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
		IP:   ip,
		Port: dhcpv4.ServerPort,
	})
	if err != nil {
		return nil, err
	}

	rawConn, err := raw.ListenPacket(&iface, syscall.ETH_P_IP, &raw.Config{LinuxSockDGRAM: false})
	if err != nil {
		udpConn.Close()
		return nil, err
	}

	c := &listener{
		rawConn:  rawConn,
		udpConn:  udpConn,
		iface:    iface,
		ip:       ip,
		requests: make(chan *Request, 10),
	}

	go c.discardUDPInput()
	go c.receiveRaw()

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
	err := c.udpConn.Close()

	if e := c.rawConn.Close(); e != nil && err == nil {
		err = e
	}

	return err
}

func (c *listener) Raw() net.PacketConn {
	return c.rawConn
}

func (c *listener) UDP() net.PacketConn {
	return c.udpConn
}

func (c *listener) Recv(ctx context.Context) (*Request, error) {
	select {
	case v := <-c.requests:
		v.Iface = c.iface
		return v, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *listener) discardUDPInput() {
	b := make([]byte, 4096)
	for {
		_, _, err := c.udpConn.ReadFrom(b)
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

func (c *listener) receiveRaw() {
	b := make([]byte, 4096)
	defer close(c.requests)

	for {
		n, peer, err := c.rawConn.ReadFrom(b)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Timeout() || opErr.Temporary() {
					continue
				}
			}
			return
		}

		packet := gopacket.NewPacket(b[:n], layers.LayerTypeEthernet, gopacket.Default)
		if err := packet.ErrorLayer(); err != nil {
			//log.Println("failed to decode packet", err)
			continue
		}

		ipLayer, ok := packet.NetworkLayer().(*layers.IPv4)
		if !ok {
			//log.Println(peerHwAddr, "not an IPv4 packet")
			continue
		}

		srcIP := ipLayer.SrcIP.To4()

		udpLayer, ok := packet.TransportLayer().(*layers.UDP)
		if !ok {
			//log.Println(peerHwAddr, srcIP, ipLayer.DstIP, "not a UDP packet", ipLayer.Protocol)
			continue
		}

		if udpLayer.DstPort != dhcpv4.ServerPort {
			//log.Println(peerHwAddr, srcIP, ipLayer.DstIP, "not sent to server port", udpLayer.DstPort)
			continue
		}

		if len(udpLayer.Payload) == 0 {
			//log.Println("no packet payload ...")
			continue
		}

		dhcpRequest, err := dhcpv4.FromBytes(udpLayer.Payload)
		if err != nil {
			log.Println("malformed DHCP request message: ", err)
			continue
		}

		c.requests <- &Request{
			Peer: &net.UDPAddr{
				IP:   srcIP,
				Port: int(udpLayer.SrcPort),
			},
			PeerHwAddr: peer.(*raw.Addr).HardwareAddr,
			Message:    dhcpRequest,
		}
	}
}
