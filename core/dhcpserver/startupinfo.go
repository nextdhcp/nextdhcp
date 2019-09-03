package dhcpserver

import "fmt"

func getStartupInfo(cfg []*Config) string {
	s := ""

	for _, c := range cfg {
		s += fmt.Sprintf("\t%s on %s (%s)\n", c.Network.String(), c.IP, c.Interface.Name)
	}

	if s != "" {
		s = "Serving the following subnets\n" + s
	}

	return s
}
