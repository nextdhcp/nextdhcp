package lease

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLease(t *testing.T) {
	l := Lease{
		Client: Client{
			ID:       "client",
			Hostname: "client-hostname",
			HwAddr:   net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0xaa},
		},
		Expires: time.Now().Add(time.Minute),
		Address: net.IP{10, 0, 0, 1},
	}

	t.Run("ExpiredAt", func(t *testing.T) {
		assert.False(t, l.Expired())
		assert.False(t, l.ExpiredAt(time.Now()))
		assert.True(t, l.ExpiredAt(time.Now().Add(time.Hour)))
	})

	t.Run("Clone", func(t *testing.T) {
		c := l.Clone()
		assert.Equal(t, l.Expires.Unix(), c.Expires.Unix())
		assert.Equal(t, l.Address.String(), c.Address.String())
		c.Address[0] = 255
		assert.NotEqual(t, l.Address, c.Address)

		assert.Equal(t, l.Client, c.Client)
		c.Client.HwAddr[0] = 255
		assert.NotEqual(t, l.HwAddr, c.HwAddr)
	})
}
