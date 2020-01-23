package iprange

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		// invalid ranges
		{
			IPRange{
				Start: nil,
				End:   net.ParseIP("10.0.1.100"),
			},
			0,
		},
		{
			IPRange{
				Start: net.ParseIP("10.0.1.10"),
				End:   net.IP{0, 1},
			},
			0,
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
		res := Merge(c.I)
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

		// #3
		{
			I: []*IPRange{
				{
					Start: net.IP{10, 8, 0, 10},
					End:   net.IP{10, 8, 0, 100},
				},
			},
			D: &IPRange{
				Start: net.IP{10, 8, 0, 20},
				End:   net.IP{10, 8, 0, 100},
			},
			E: []*IPRange{
				{
					Start: net.IP{10, 8, 0, 10},
					End:   net.IP{10, 8, 0, 19},
				},
			},
		},
	}

	for i, c := range cases {
		res := DeleteFrom(c.D, c.I)
		assert.Equal(t, c.E, res, "Test case #%d failed", i)
	}
}

func TestIPRange_ByIdx(t *testing.T) {
	r := IPRange{
		Start: net.IP{10, 8, 0, 10},
		End:   net.IP{10, 8, 0, 20},
	}

	assert.Equal(t, net.IP{10, 8, 0, 10}, r.ByIdx(0))
	assert.Equal(t, net.IP{10, 8, 0, 15}, r.ByIdx(5))
	assert.Equal(t, net.IP{10, 8, 0, 20}, r.ByIdx(10))

	// invalid range
	r = IPRange{
		Start: nil,
		End:   net.IP{10, 8, 0, 20},
	}

	assert.Nil(t, r.ByIdx(1))
}

func TestIPRange_String(t *testing.T) {
	r := IPRange{
		Start: net.IP{10, 8, 0, 10},
		End:   net.IP{10, 8, 0, 20},
	}

	assert.Equal(t, "10.8.0.10-10.8.0.20", r.String())
}

func TestIPRange_Validate(t *testing.T) {
	r := IPRange{
		Start: net.IP{10, 8, 0, 10},
		End:   net.IP{10, 8, 0, 20},
	}

	assert.NoError(t, r.Validate())

	r.Start = nil
	require.Error(t, r.Validate())
	assert.Equal(t, "Invalid start IP", r.Validate().Error())

	r.Start = net.IP{10, 8, 0, 10}
	r.End = nil
	require.Error(t, r.Validate())
	assert.Equal(t, "Invalid end IP", r.Validate().Error())

	r.End = net.IP{10, 8, 0, 1}
	r.Start = net.IP{10, 8, 0, 10}
	require.Error(t, r.Validate())
	assert.Equal(t, "Invalid range", r.Validate().Error())
}

func TestIPRanges(t *testing.T) {
	ranges := IPRanges{
		&IPRange{
			Start: net.IP{10, 8, 0, 10},
			End:   net.IP{10, 8, 0, 20},
		},
		&IPRange{
			Start: net.IP{10, 8, 0, 100},
			End:   net.IP{10, 8, 0, 102},
		},
	}

	assert.Equal(t, "10.8.0.10-10.8.0.20, 10.8.0.100-10.8.0.102", ranges.String())
	assert.True(t, ranges.Contains(net.IP{10, 8, 0, 10}))
	assert.True(t, ranges.Contains(net.IP{10, 8, 0, 15}))
	assert.False(t, ranges.Contains(net.IP{10, 8, 0, 21}))
	assert.True(t, ranges.Contains(net.IP{10, 8, 0, 101}))
}
