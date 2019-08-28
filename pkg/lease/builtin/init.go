package builtin

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ppacher/dhcp-ng/pkg/lease"
	"github.com/ppacher/dhcp-ng/pkg/lease/iprange"
)

func factory(opts map[string]interface{}) (lease.Database, error) {
	network, ok := opts["network"]
	if !ok {
		return nil, errors.New("missing `network` option")
	}

	var ipNet net.IPNet
	if n, ok := network.(net.IPNet); ok {
		ipNet = n
	} else if n, ok := network.(*net.IPNet); ok {
		ipNet = *n
	} else if s, ok := network.(string); ok {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}

		ipNet = *n
	} else {
		return nil, errors.New("invalid type for `network` option")
	}

	var ranges []*iprange.IPRange

	rangesOption, ok := opts["ranges"]
	if ok {
		// TODO(ppacher) support a better range definition format
		s, ok := rangesOption.([]interface{})
		if !ok {
			return nil, errors.New("invalid type for `ranges` option")
		}

		for idx, v := range s {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("invalid type for `ranges` value at index %d", idx)
			}

			parts := strings.Split(s, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid format for `ranges` value at index %d", idx)
			}

			start := strings.TrimSpace(parts[0])
			end := strings.TrimSpace(parts[1])

			startIP := net.ParseIP(start)
			endIP := net.ParseIP(end)

			if startIP == nil || endIP == nil {
				return nil, fmt.Errorf("invalid format for `ranges` value at index %d", idx)
			}

			ranges = append(ranges, &iprange.IPRange{
				Start: startIP,
				End:   endIP,
			})
		}
	}

	return New(&ipNet, ranges), nil
}

func init() {
	lease.MustRegisterDriver("", factory)
}
