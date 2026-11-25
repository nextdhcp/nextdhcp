package dhcpserver

import "fmt"

func getStartupInfo(servers []*Server) string {
	s := ""

	for _, c := range servers {
		s += fmt.Sprintf("\t%s on %s (%s)\n", c.Network.String(), c.IP, c.iface.Name)
	}

	if s != "" {
		s = "Serving the following subnets\n" + s
	}

	return s
}
