package core

import (
	// Plugin the dhcpserver
	_ "github.com/ppacher/dhcp-ng/core/dhcpserver"
	
	// And all all core directives
	_ "github.com/ppacher/dhcp-ng/core/directives"
)