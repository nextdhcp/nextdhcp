package iprange

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// IPRange is a range of IP address from (inclusive) start to (inclusive)
// end IP.
type IPRange struct {
	Start net.IP
	End   net.IP
}

// Len returns the number of IP address available inside the range
func (r *IPRange) Len() int {
	if r == nil {
		return 0
	}

	end4, ok := IP2Int(r.End)
	if !ok {
		return 0
	}

	start4, ok := IP2Int(r.Start)
	if !ok {
		return 0
	}

	return int(end4) - int(start4) + 1
}

func (r *IPRange) String() string {
	return fmt.Sprintf("%s-%s", r.Start, r.End)
}

// ByIdx returns the IP address at the given index
func (r *IPRange) ByIdx(i int) net.IP {
	start, ok := IP2Int(r.Start)
	if !ok {
		return nil
	}

	return Int2IP(start + uint32(i))
}

// Contains checks if ip is part of the range
func (r *IPRange) Contains(ip net.IP) bool {
	ipV4 := ip.To4()
	if ipV4 == nil {
		return false
	}

	start := binary.BigEndian.Uint32(r.Start.To4())
	end := binary.BigEndian.Uint32(r.End.To4())
	x := binary.BigEndian.Uint32(ipV4)

	return start <= x && x <= end
}

// Clone returns a deep copy of the IP range
func (r *IPRange) Clone() *IPRange {
	start := append(net.IP{}, r.Start...)
	end := append(net.IP{}, r.End...)

	return &IPRange{start, end}
}

// IP2Int converts a IPv4 address to it's unsigned integer representation
func IP2Int(ip net.IP) (uint32, bool) {
	v4 := ip.To4()
	if v4 == nil {
		return 0, false
	}

	return binary.BigEndian.Uint32(v4), true
}

// Int2IP converts a uint32 to it's IPv4 representation
func Int2IP(i uint32) net.IP {
	r := make([]byte, 4)
	binary.BigEndian.PutUint32(r, i)
	return net.IPv4(r[0], r[1], r[2], r[3]).To4()
}

func nextIP(ip net.IP) net.IP {
	n, ok := IP2Int(ip)
	if !ok {
		return nil
	}

	n = n + 1
	r := make([]byte, len(ip))

	binary.BigEndian.PutUint32(r, n)
	return net.IPv4(r[0], r[1], r[2], r[3]).To4()
}

func prevIP(ip net.IP) net.IP {
	n, ok := IP2Int(ip)
	if !ok {
		return nil
	}

	n = n - 1
	r := make([]byte, len(ip))

	binary.BigEndian.PutUint32(r, n)
	return net.IPv4(r[0], r[1], r[2], r[3]).To4()
}

// Validate the IP range and return any error encountered
func (r *IPRange) Validate() error {
	start4, startOk := IP2Int(r.Start)
	end4, endOk := IP2Int(r.End)

	if !startOk {
		return errors.New("invalid start IP")
	}

	if !endOk {
		return errors.New("invalid end IP")
	}

	if start4 >= end4 {
		return errors.New("invalid range")
	}

	return nil
}

// IPRanges is a slice of IPRange and implements the sort.Interface.
// Ranges are sorted by increasing start IP
type IPRanges []*IPRange

// Len implements sort.Interface
func (ranges IPRanges) Len() int {
	return len(ranges)
}

// Less implements sort.Interface
func (ranges IPRanges) Less(i, j int) bool {
	startI, _ := IP2Int(ranges[i].Start)
	startJ, _ := IP2Int(ranges[j].Start)

	return startI < startJ
}

// Swap implements sort.Interface
func (ranges IPRanges) Swap(i, j int) {
	t := ranges[i]
	ranges[i] = ranges[j]
	ranges[j] = t
}

// Contains reports whether on of the IP ranges contains the
// IP in question
func (ranges IPRanges) Contains(ip net.IP) bool {
	for _, r := range ranges {
		if r.Contains(ip) {
			return true
		}
	}

	return false
}

func (ranges IPRanges) String() string {
	s := make([]string, 0, len(ranges))

	for _, r := range ranges {
		s = append(s, r.String())
	}

	return strings.Join(s, ", ")
}

// Merge merges multiple IP ranges and combins overlapping ranges
func Merge(ranges []*IPRange) []*IPRange {
	if len(ranges) == 0 {
		return nil
	}

	stack := []*IPRange{}

	// sort ranges in increasing order of start IP
	sort.Sort(IPRanges(ranges))

	// push the very first entry onto the merged stack
	stack = append(stack, ranges[0].Clone())

	// start from the second range
	for i := 1; i < len(ranges); i++ {
		top := stack[len(stack)-1]

		topEnd, _ := IP2Int(top.End)
		curStart, _ := IP2Int(ranges[i].Start)
		curEnd, _ := IP2Int(ranges[i].End)

		// push onto stack if we are not overlapping with stack top
		if topEnd < curStart {
			stack = append(stack, ranges[i].Clone())
		} else if topEnd < curEnd {
			// otherwise update the ending time if we have a "bigger" end IP
			top.End = append(net.IP{}, ranges[i].End...)
		}
	}

	return stack
}

// DeleteFrom deletes a range from a set of IP ranges. The range to delte may span multiple
// ip ranges
func DeleteFrom(delete *IPRange, ranges []*IPRange) []*IPRange {
	stack := []*IPRange{}

	deleteStart, _ := IP2Int(delete.Start)
	deleteEnd, _ := IP2Int(delete.End)

	for i := 0; i < len(ranges); i++ {
		currStart, _ := IP2Int(ranges[i].Start)
		currEnd, _ := IP2Int(ranges[i].End)

		// skip ranges that cannot contain the range to delete
		if deleteStart > currEnd {
			continue
		}

		// do an early exit if no more matching range
		// can exist
		if deleteEnd < currStart {
			break
		}

		startInRange := deleteStart >= currStart && deleteStart <= currEnd
		endInRange := deleteEnd >= currStart && deleteEnd <= currEnd

		// not in range: append and continue
		if !startInRange && !endInRange {
			stack = append(stack, ranges[i])
			continue
		}

		// if true, cut down the end IP of the current range
		if startInRange {
			r := &IPRange{
				Start: ranges[i].Start,
				End:   prevIP(delete.Start), // - 1
			}

			if r.Len() > 0 {
				stack = append(stack, r)
			}
		}

		if endInRange {
			r := &IPRange{
				Start: nextIP(delete.End), // + 1
				End:   ranges[i].End,
			}

			if r.Len() > 0 {
				stack = append(stack, r)
			}
		}

	}

	return stack
}
