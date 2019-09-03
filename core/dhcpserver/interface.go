package dhcpserver

import (
	"net"
	"fmt"
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
