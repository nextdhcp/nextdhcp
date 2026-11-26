package dhcpserver

import "fmt"

func getStartupInfo(servers []*Server) string {
	s := ""

	for _, c := range servers {
		transport := ""
		if c.transport() == TransportUDP {
			transport = ", udp transport"
		}

		s += fmt.Sprintf("\t%s on %s (%s%s)\n", c.Network.String(), c.IP, c.iface.Name, transport)
	}

	if s != "" {
		s = "Serving the following subnets\n" + s
	}

	return s
}
