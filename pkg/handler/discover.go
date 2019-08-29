package handler

import (
	"context"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/ppacher/dhcp-ng/pkg/lease"
)

// prepareDHCPv4Offer prepares a new DHCP IP address offer for the given DHCP request
func prepareDHCPv4Offer(ctx context.Context, req *dhcpv4.DHCPv4, s *SubnetConfig) (*dhcpv4.DHCPv4, error) {

	cli := lease.Client{
		HwAddr:   req.ClientHWAddr,
		Hostname: req.HostName(),
	}

	var ip net.IP
	var err error

	// TODO(ppacher): the below code fails if we failed to find a DHCP IP address
	// that we can lease to the requesting client. We may should try in a loop
	// (with a reasonable maxTries) since we can race with other goroutines
	// between FindAddress() and Reserve()

	ip = req.RequestedIPAddress()
	if ip == nil {
		if ip, err = s.Database.FindAddress(ctx, &cli); err != nil {
			return nil, err
		}
	}

	if err := s.Database.Reserve(ctx, ip, cli); err != nil {
		if req.RequestedIPAddress() == nil {
			return nil, err
		}

		// we tried to get the IP address that the client requested
		// no try to find a new one
		if ip, err = s.Database.FindAddress(ctx, &cli); err != nil {
			return nil, err
		}

		if err := s.Database.Reserve(ctx, ip, cli); err != nil {
			return nil, err
		}
	}

	resp, err := dhcpv4.NewReplyFromRequest(req,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer),
		dhcpv4.WithYourIP(ip),
		dhcpv4.WithServerIP(s.IP),
		dhcpv4.WithNetmask(s.Network.Mask),
	)
	if err != nil {
		return nil, err
	}

	resp.UpdateOption(dhcpv4.OptServerIdentifier(s.IP))
	resp.UpdateOption(dhcpv4.OptIPAddressLeaseTime(s.LeaseTime))

	for code, value := range s.Options {
		// TODO(ppacher): check which option SHOULD NOT be set on DHCP offers
		resp.UpdateOption(dhcpv4.OptGeneric(code, value.ToBytes()))
	}

	return resp, nil
}
