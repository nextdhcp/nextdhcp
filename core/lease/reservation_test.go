package lease

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReservedAddressExpired(t *testing.T) {
	expires := time.Now().Add(time.Minute)
	r := ReservedAddress{
		Client: Client{
			Hostname: "hostname",
			HwAddr:   net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
		IP:      net.IP{192, 168, 0, 1},
		Expires: &expires,
	}

	assert.True(t, r.Expired(time.Now().Add(time.Hour)))
	assert.False(t, r.Expired(time.Now()))

	// Should never expire
	r.Expires = nil
	assert.False(t, r.Expired(time.Now()))
	assert.False(t, r.Expired(time.Now().Add(time.Hour*24*356)))
}

func makeReservedAddressList() ReservedAddressList {
	list := []ReservedAddress{}
	for i := 0; i < 10; i++ {
		expires := time.Now().Add(time.Duration(i) * time.Minute)
		list = append(list, ReservedAddress{
			Client: Client{
				Hostname: fmt.Sprintf("host-%d", i),
				ID:       fmt.Sprintf("id-%d", i),
				HwAddr:   net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, byte(i)},
			},
			IP:      net.IP{10, 0, 0, byte(i)},
			Expires: &expires,
		})
	}

	return list
}

func TestReservedAddressList(t *testing.T) {
	cases := []struct {
		i byte
		e bool
	}{
		{1, true},
		{5, true},
		{20, false},
	}
	list := makeReservedAddressList()

	t.Run("FindIP", func(t *testing.T) {
		for _, c := range cases {
			ip := net.IP{10, 0, 0, c.i}

			if c.e {
				assert.NotNil(t, list.FindIP(ip))
			} else {
				assert.Nil(t, list.FindIP(ip))
			}
		}
	})

	t.Run("FindMAC", func(t *testing.T) {
		for _, c := range cases {
			hw := net.HardwareAddr{0, 0, 0, 0, 0, c.i}

			if c.e {
				assert.NotNil(t, list.FindMAC(hw))
			} else {
				assert.Nil(t, list.FindMAC(hw))
			}
		}
	})

	t.Run("FindHostname", func(t *testing.T) {
		for _, c := range cases {
			host := fmt.Sprintf("host-%d", c.i)

			if c.e {
				assert.NotNil(t, list.FindHostname(host))
			} else {
				assert.Nil(t, list.FindHostname(host))
			}
		}
	})

	t.Run("FindID", func(t *testing.T) {
		for _, c := range cases {
			host := fmt.Sprintf("id-%d", c.i)

			if c.e {
				assert.NotNil(t, list.FindID(host))
			} else {
				assert.Nil(t, list.FindID(host))
			}
		}
	})
}
