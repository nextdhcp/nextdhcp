package option

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/insomniacslk/dhcp/rfc1035label"
)

// ToString returns the string represenation of data interpreted by code.
// This method has been copied (with slight modifications) from insomniacslk/dhcp/dhcpv4
func ToString(code dhcpv4.OptionCode, data []byte, vendorDecoder dhcpv4.OptionDecoder) string {
	var d dhcpv4.OptionDecoder
	switch code {
	case dhcpv4.OptionRouter, dhcpv4.OptionDomainNameServer, dhcpv4.OptionNTPServers, dhcpv4.OptionServerIdentifier:
		d = &dhcpv4.IPs{}

	case dhcpv4.OptionBroadcastAddress, dhcpv4.OptionRequestedIPAddress:
		d = &dhcpv4.IP{}

	case dhcpv4.OptionClientSystemArchitectureType:
		d = &iana.Archs{}

	case dhcpv4.OptionSubnetMask:
		d = &dhcpv4.IPMask{}

	case dhcpv4.OptionDHCPMessageType:
		var mt dhcpv4.MessageType
		d = &mt

	case dhcpv4.OptionParameterRequestList:
		d = &dhcpv4.OptionCodeList{}

	case dhcpv4.OptionHostName, dhcpv4.OptionDomainName, dhcpv4.OptionRootPath,
		dhcpv4.OptionClassIdentifier, dhcpv4.OptionTFTPServerName, dhcpv4.OptionBootfileName:
		var s dhcpv4.String
		d = &s

	case dhcpv4.OptionRelayAgentInformation:
		d = &dhcpv4.RelayOptions{}

	case dhcpv4.OptionDNSDomainSearchList:
		d = &rfc1035label.Labels{}

	case dhcpv4.OptionIPAddressLeaseTime:
		var dur dhcpv4.Duration
		d = &dur

	case dhcpv4.OptionMaximumDHCPMessageSize:
		var u dhcpv4.Uint16
		d = &u

	case dhcpv4.OptionUserClassInformation:
		var s dhcpv4.Strings
		d = &s
		if s.FromBytes(data) != nil {
			var s dhcpv4.String
			d = &s
		}

	case dhcpv4.OptionVendorIdentifyingVendorClass:
		d = &dhcpv4.VIVCIdentifiers{}

	case dhcpv4.OptionVendorSpecificInformation:
		d = vendorDecoder

	case dhcpv4.OptionClasslessStaticRoute:
		d = &dhcpv4.Routes{}
	}
	if d != nil && d.FromBytes(data) == nil {
		return d.String()
	}
	return dhcpv4.OptionGeneric{Data: data}.String()
}
