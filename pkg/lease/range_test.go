package lease

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IPRange_Len(t *testing.T) {
	cases := []struct {
		I IPRange
		E int
	}{
		{
			IPRange{
				Start: net.ParseIP("10.0.0.0"),
				End:   net.ParseIP("10.0.0.0"),
			},
			0,
		},
		{
			IPRange{
				Start: net.ParseIP("10.0.0.0"),
				End:   net.ParseIP("10.0.0.100"),
			},
			100,
		},
		{
			IPRange{
				Start: net.ParseIP("10.0.0.0"),
				End:   net.ParseIP("10.0.1.100"),
			},
			356,
		},
	}

	for i, c := range cases {
		assert.Equal(t, c.E, c.I.Len(), "Test case #%d failed", i)
	}
}

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

func Test_mergeConsecutiveRanges(t *testing.T) {
	cases := []struct {
		I []*IPRange
		E []*IPRange
	}{
		// #1 An empty range should return nil
		{
			[]*IPRange{},
			nil,
		},
		// #2 An empty range should return nil
		{
			nil,
			nil,
		},

		// #3 One range should be returned as it is
		{
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.10"),
				},
			},
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.10"),
				},
			},
		},

		// #4 Extend END of first range
		{
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.10"),
				},
				{
					Start: net.ParseIP("192.168.0.8"),
					End:   net.ParseIP("192.168.0.100"),
				},
			},
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.100"),
				},
			},
		},

		// #5 Sorted and merged correctly
		{
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.9"),
					End:   net.ParseIP("192.168.0.100"),
				},
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.9"),
				},
			},
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.1"),
					End:   net.ParseIP("192.168.0.100"),
				},
			},
		},

		// #6 Sorted but not merged
		{
			[]*IPRange{
				{
					Start: net.ParseIP("192.168.0.9"),
					End:   net.ParseIP("192.168.0.100"),
				},
				{
					Start: net.ParseIP("10.8.0.1"),
					End:   net.ParseIP("10.9.0.9"),
				},
			},
			[]*IPRange{
				{
					Start: net.ParseIP("10.8.0.1"),
					End:   net.ParseIP("10.9.0.9"),
				},
				{
					Start: net.ParseIP("192.168.0.9"),
					End:   net.ParseIP("192.168.0.100"),
				},
			},
		},
	}

	for i, c := range cases {
		res := mergeConsecutiveRanges(c.I)
		assert.Equal(t, c.E, res, "Test case #%d failed", i)
	}
}

func Test_deleteRange(t *testing.T) {
	cases := []struct {
		I []*IPRange
		D *IPRange
		E []*IPRange
	}{
		// #0
		{
			I: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.1").To4(),
					End:   net.ParseIP("10.8.0.100").To4(),
				},
			},
			D: &IPRange{
				Start: net.ParseIP("10.8.0.10").To4(),
				End:   net.ParseIP("10.8.0.101").To4(),
			},
			E: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.1").To4(),
					End:   net.ParseIP("10.8.0.9").To4(),
				},
			},
		},

		// #1
		{
			I: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.1").To4(),
					End:   net.ParseIP("10.8.0.100").To4(),
				},
			},
			D: &IPRange{
				Start: net.ParseIP("10.8.0.10").To4(),
				End:   net.ParseIP("10.8.0.20").To4(),
			},
			E: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.1").To4(),
					End:   net.ParseIP("10.8.0.9").To4(),
				},
				{
					Start: net.ParseIP("10.8.0.21").To4(),
					End:   net.ParseIP("10.8.0.100").To4(),
				},
			},
		},

		// #2
		{
			I: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.10").To4(),
					End:   net.ParseIP("10.8.0.100").To4(),
				},
			},
			D: &IPRange{
				Start: net.ParseIP("10.8.0.1").To4(),
				End:   net.ParseIP("10.8.0.20").To4(),
			},
			E: []*IPRange{
				{
					Start: net.ParseIP("10.8.0.21").To4(),
					End:   net.ParseIP("10.8.0.100").To4(),
				},
			},
		},
	}

	for i, c := range cases {
		res := deleteRange(c.D, c.I)
		assert.Equal(t, c.E, res, "Test case #%d failed", i)
	}
}
