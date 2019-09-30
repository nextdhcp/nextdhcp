package dhcpserver

import (
	"fmt"
	"net"
)

func findInterface(cfg *Config) bool {
	if cfg.Interface.Name != "" && len(cfg.Interface.HardwareAddr) > 0 {
		return true
	}

	iface, err := findInterfaceByIP(cfg.IP)
	if err != nil {
		//log.Println(err.Error())
		return false
	}

	cfg.Interface = *iface
	return true
}

// findInterfaceByIP searches for the network interface that has
// ip assigned to it. The IP address must be the same, IPs in
// the same subnet do not count as a match
func findInterfaceByIP(ip net.IP) (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			//log.Println(iface.Name, a)

			if ipNet.IP.Equal(ip) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to find interface for %s", ip.String())
}

// findInterfaceContainingIPs searches for the network interface that
// contains the given IP address in one of it's attached local networks
func findInterfaceContainingIP(ip net.IP) (*net.Interface, *net.IPNet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, nil, err
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.Contains(ip) {
				return &iface, ipNet, nil
			}

		}
	}

	return nil, nil, fmt.Errorf("failed to find interface with %s", ip.String())
}

// tryInterfaceNameOrIP first tries to parse a CIDR IP subnet
// notation in value and will fill the IP and IPNet values of
// cfg accordingly. If value is not a valid CIDR notation
// it will assume value is the name of the interface and will
// lookup the IP configuration there. If that fails too, an
// error is returned
func tryInterfaceNameOrIP(value string, cfg *Config) error {
	ip, ipNet, err := net.ParseCIDR(value)
	if err == nil {
		cfg.IP = ip
		cfg.Network = *ipNet
		return nil
	}

	iface, err := net.InterfaceByName(value)
	if err != nil {
		return err
	}

	addr, err := iface.Addrs()
	if err != nil {
		return err
	}

	foundIPv4 := false

	for _, a := range addr {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			// not an IPNet, skip this one
			continue
		}

		if ipn.IP.To4() == nil {
			// not an IPv4 network
			continue
		}

		if foundIPv4 {
			return fmt.Errorf("interface names can only be used with one subnet assigned")
		}

		foundIPv4 = true

		ip = ipn.IP
		ipNet = ipn
	}

	if !foundIPv4 {
		return fmt.Errorf("no usable subnet found")
	}

	cfg.IP = ip
	cfg.Network = *ipNet

	return nil
}
