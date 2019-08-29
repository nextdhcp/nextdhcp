package handler

import (
	"context"
	"fmt"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func handleDHCPv4Release(ctx context.Context, req *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error) {
	return nil, fmt.Errorf("not yet supported")
}
