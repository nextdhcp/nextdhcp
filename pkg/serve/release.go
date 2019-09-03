package serve

import (
	"fmt"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/pkg/middleware"
)

func handleDHCPv4Release(ctx *middleware.Context, req *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error) {
	return nil, fmt.Errorf("not yet supported")
}
