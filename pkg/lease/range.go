package lease

import (
	"encoding/binary"
	"errors"
	"net"
)

// IPRange is a range of IP address from (inclusive) start to (inclusive)
// end IP.
type IPRange struct {
	Start net.IP
	End   net.IP
}

// Len returns the number of IP address available inside the range
func (r *IPRange) Len() int {
	// TODO(ppacher): should we try to find the containing network CIDR and check
	// if start/end contain network/broadcast addresses
	return int(binary.BigEndian.Uint32(r.End.To4()) - binary.BigEndian.Uint32(r.Start.To4()))
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

// IPToInt converts a IPv4 address to it's unsigned integer representation
func IPToInt(ip net.IP) (uint32, bool) {
	v4 := ip.To4()
	if v4 == nil {
		return 0, false
	}

	return binary.BigEndian.Uint32((v4)), true
}

func (r *IPRange) Validate() error {
	start4, startOk := IPToInt(r.Start)
	end4, endOk := IPToInt(r.End)

	if !startOk || !endOk {
		return errors.New("")
	}
	return nil
}
