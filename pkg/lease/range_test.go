package lease

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IPRange_Contains(t *testing.T) {
	r := IPRange{
		Start: net.IPv4(192, 168, 0, 100),
		End:   net.IPv4(192, 168, 2, 10),
	}

	cases := []struct {
		IP string
		E  bool
	}{
		{
			IP: "192.168.0.100",
			E:  true,
		},
		{
			IP: "192.168.2.10",
			E:  true,
		},
		{
			IP: "192.168.3.100",
			E:  false,
		},
		{
			IP: "1.1.1.1",
			E:  false,
		},
		{
			IP: "192.168.1.0",
			E:  true,
		},
	}

	for i, c := range cases {
		ip := net.ParseIP(c.IP)

		assert.Equal(t, c.E, r.Contains(ip), "Test case #%d failed", i)
	}
}
