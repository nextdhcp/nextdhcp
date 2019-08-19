package lua

import (
	"errors"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	lua "github.com/yuin/gopher-lua"
)

// StringFactory converts a string to an option
type StringFactory func(s string) (dhcpv4.OptionValue, error)

// StringListFactory converts a string slice to an option
type StringListFactory func(s []string) (dhcpv4.OptionValue, error)

// NumberFactory converts a number to an option
type NumberFactory func(x float64) (dhcpv4.OptionValue, error)

// NumberListFactory converts a number slice to an option
type NumberListFactory func(x []float64) (dhcpv4.OptionValue, error)

// ToLuaFunc is a function coverting a dhcpv4.OptionValue to a string representation
type ToLuaFunc func(*lua.LState, dhcpv4.OptionValue) (lua.LValue, error)

// KnownType defines a Lua-to-OptionValue and an OptionValue-to-Lua conversion
// function
type KnownType struct {
	ToValue   interface{}
	FromValue ToLuaFunc
}

type OptionType struct {
	Factory    interface{}
	Code       dhcpv4.OptionCode
	ToLuaValue ToLuaFunc
}

func ipOption(s string) (dhcpv4.OptionValue, error) {
	i := net.ParseIP(s)
	if i == nil {
		return nil, errors.New("invalid IP address")
	}

	return dhcpv4.IP(i), nil
}

func ipToLua(_ *lua.LState, x dhcpv4.OptionValue) (lua.LValue, error) {
	return lua.LString(x.(dhcpv4.IP).String()), nil
}

func ipListOption(s []string) (dhcpv4.OptionValue, error) {
	var ips []net.IP

	for _, i := range s {
		ip := net.ParseIP(i)
		if ip == nil {
			return nil, errors.New("invalid IP address")
		}

		ips = append(ips, ip)
	}

	return dhcpv4.IPs(ips), nil
}

func ipListToLua(l *lua.LState, x dhcpv4.OptionValue) (lua.LValue, error) {
	tbl := l.NewTable()

	for _, ip := range x.(dhcpv4.IPs) {
		tbl.Append(lua.LString(ip.String()))
	}

	return tbl, nil
}

func stringOption(s string) (dhcpv4.OptionValue, error) {
	return dhcpv4.String(s), nil
}

func stringToLua(_ *lua.LState, x dhcpv4.OptionValue) (lua.LValue, error) {
	return lua.LString(x.(dhcpv4.String)), nil
}

func stringListOption(s []string) (dhcpv4.OptionValue, error) {
	return dhcpv4.Strings(s), nil
}

func stringsToLua(l *lua.LState, x dhcpv4.OptionValue) (lua.LValue, error) {
	tbl := l.NewTable()

	for _, s := range x.(dhcpv4.Strings) {
		tbl.Append(lua.LString(s))
	}

	return tbl, nil
}

func durationOption(s string) (dhcpv4.OptionValue, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, err
	}

	return dhcpv4.Duration(d), nil
}

var (
	// TypeIP represents an IP type
	TypeIP = &KnownType{ipOption, ipToLua}

	// TypeIPList represents a list of IP addresses
	TypeIPList = &KnownType{ipListOption, ipListToLua}

	// TypeString represents a String type
	TypeString = &KnownType{stringOption, stringToLua}

	// TypeStringList represents a list of strings
	TypeStringList = &KnownType{stringListOption, stringsToLua}
)

var typeKeyToType = map[string]*KnownType{
	"TYPE_IP":          TypeIP,
	"TYPE_IP_LIST":     TypeIPList,
	"TYPE_STRING":      TypeString,
	"TYPE_STRING_LIST": TypeStringList,
}

var optionNames = map[string]dhcpv4.OptionCode{
	// IP list options
	"router":            dhcpv4.OptionRouter,
	"nameserver":        dhcpv4.OptionDomainNameServer,
	"ntp_server":        dhcpv4.OptionNTPServers,
	"server_identifier": dhcpv4.OptionServerIdentifier,

	// IP options
	"broadcast_address": dhcpv4.OptionBroadcastAddress,
	"requested_ip":      dhcpv4.OptionRequestedIPAddress,
	"netmask":           dhcpv4.OptionSubnetMask,

	// String options
	"host_name":        dhcpv4.OptionHostName,
	"domain_name":      dhcpv4.OptionDomainName,
	"root_path":        dhcpv4.OptionRootPath,
	"class_identifier": dhcpv4.OptionClassIdentifier,
	"tftp_server_name": dhcpv4.OptionTFTPServerName,
	"filename":         dhcpv4.OptionBootfileName,

	// leaseTime
	"leaseTime": dhcpv4.OptionIPAddressLeaseTime,

	// strings
	"user_class_information": dhcpv4.OptionUserClassInformation,
}

var optionTypes = map[dhcpv4.OptionCode]*KnownType{
	// IP list options
	dhcpv4.OptionRouter:           TypeIPList,
	dhcpv4.OptionDomainNameServer: TypeIPList,
	dhcpv4.OptionNTPServers:       TypeIPList,
	dhcpv4.OptionServerIdentifier: TypeIPList,

	// IP options
	dhcpv4.OptionBroadcastAddress:   TypeIP,
	dhcpv4.OptionRequestedIPAddress: TypeIP,
	dhcpv4.OptionSubnetMask:         TypeIP,

	// String options
	dhcpv4.OptionHostName:        TypeString,
	dhcpv4.OptionDomainName:      TypeString,
	dhcpv4.OptionRootPath:        TypeString,
	dhcpv4.OptionClassIdentifier: TypeString,
	dhcpv4.OptionTFTPServerName:  TypeString,
	dhcpv4.OptionBootfileName:    TypeString,

	// leaseTime
	//"leaseTime": dhcpv4.OptionIPAddressLeaseTime,

	// strings
	dhcpv4.OptionUserClassInformation: TypeStringList,
}

// GetOptionTypeMap returns a map of dhcpv4 option-code to KnownType
func GetOptionTypeMap() map[dhcpv4.OptionCode]*KnownType {
	m := make(map[dhcpv4.OptionCode]*KnownType)
	for key, value := range optionTypes {
		m[key] = &(*value)
	}

	return m
}
