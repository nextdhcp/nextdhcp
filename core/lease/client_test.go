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
