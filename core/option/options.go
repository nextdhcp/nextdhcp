package option

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

type (
	single func(string) (dhcpv4.OptionValue, error)
	list   func([]string) (dhcpv4.OptionValue, error)
	custom func(string, []string) (dhcpv4.OptionCode, dhcpv4.OptionValue, error)
)

var (
	// ErrUnknownOption is returned from ParseKnown when the option name is not defined
	// in the list below
	ErrUnknownOption = errors.New("unknown option")

	options = map[string]dhcpv4.OptionCode{
		// IP list options
		"router":            dhcpv4.OptionRouter,
		"nameserver":        dhcpv4.OptionDomainNameServer,
		"ntp-server":        dhcpv4.OptionNTPServers,
		"server-identifier": dhcpv4.OptionServerIdentifier,

		// IP options
		"broadcast-address": dhcpv4.OptionBroadcastAddress,
		"requested-ip":      dhcpv4.OptionRequestedIPAddress,
		"netmask":           dhcpv4.OptionSubnetMask,

		// String options
		"hostname":         dhcpv4.OptionHostName,
		"domain-name":      dhcpv4.OptionDomainName,
		"root-path":        dhcpv4.OptionRootPath,
		"class-identifier": dhcpv4.OptionClassIdentifier,
		"tftp-server-name": dhcpv4.OptionTFTPServerName,
		"filename":         dhcpv4.OptionBootfileName,

		// strings
		"user-class-information": dhcpv4.OptionUserClassInformation,
	}

	optionParser = map[string]interface{}{
		// IP list options
		"router":            list(IPListOption),
		"nameserver":        list(IPListOption),
		"ntp-server":        list(IPListOption),
		"server-identifier": list(IPListOption),

		// IP options
		"broadcast-address": single(IPOption),
		"requested-ip":      single(IPOption),
		"netmask":           single(IPOption),

		// String options
		"hostname":         single(StringOption),
		"domain-name":      single(StringOption),
		"root-path":        single(StringOption),
		"class-identifier": single(StringOption),
		"tftp-server-name": single(StringOption),
		"filename":         single(StringOption),

		// strings
		"user-class-information": list(StringListOption),
	}
)

// StringOption converts the given string into a DHCPv4 option value
func StringOption(s string) (dhcpv4.OptionValue, error) {
	return dhcpv4.String(s), nil
}

// StringListOption converts the given string slice into a DHCPv4 option value
func StringListOption(s []string) (dhcpv4.OptionValue, error) {
	return dhcpv4.Strings(s), nil
}

// IPOption converts the given string into a DHCPv4 option value
func IPOption(s string) (dhcpv4.OptionValue, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address")
	}

	return dhcpv4.IP(ip), nil
}

// IPListOption converts the given string slice into a DHCPv4 option value
func IPListOption(s []string) (dhcpv4.OptionValue, error) {
	ips := make([]net.IP, 0, len(s))

	for _, i := range s {
		ip := net.ParseIP(i)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address")
		}

		ips = append(ips, ip)
	}

	return dhcpv4.IPs(ips), nil
}

// UInt16Option converts the given string into a DHCPv4 option value
func UInt16Option(s string) (dhcpv4.OptionValue, error) {
	i64, err := strconv.ParseUint(s, 0, 16)
	if err != nil {
		return nil, err
	}
	val := uint16(i64)

	return dhcpv4.Uint16(val), nil
}

// ParseKnown parses the given name and string values
// and returns their DHCP option representation if known
func ParseKnown(name string, values []string) (dhcpv4.OptionCode, dhcpv4.OptionValue, error) {
	code, ok := options[name]
	if ok {
		parser := optionParser[name]

		var val dhcpv4.OptionValue
		var err error

		switch fn := parser.(type) {
		case list:
			val, err = fn(values)
		case single:
			if len(values) > 1 {
				return nil, nil, fmt.Errorf("option %s only supports one value", name)
			}
			val, err = fn(values[0])
		case custom:
			code, val, err = fn(name, values)
		default:
			err = errors.New("unknown parser function")
		}

		if err != nil {
			return nil, nil, err
		}

		return code, val, nil
	}
	return nil, nil, ErrUnknownOption
}

// Code returns the DHCPv4 option code for the known option name
func Code(name string) (dhcpv4.OptionCode, bool) {
	code, ok := options[name]
	return code, ok
}
