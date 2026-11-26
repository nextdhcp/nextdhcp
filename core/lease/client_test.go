package lease

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientClone(t *testing.T) {
	cli := Client{
		HwAddr:   net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0xaa},
		Hostname: "client-hostname",
		ID:       "client",
	}

	c := cli.Clone()
	assert.Equal(t, cli, c)

	c.HwAddr[0] = 0xff
	assert.Equal(t, byte(0x00), cli.HwAddr[0])
}

func TestNewClient(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	cli := NewClient(mac)
	assert.Equal(t, mac, cli.HwAddr)
	assert.Equal(t, "00:aa:bb:cc:dd:ee", cli.ID)
}
