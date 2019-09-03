package core

import (
	// Plugin the dhcpserver
	_ "github.com/nextdhcp/nextdhcp/core/dhcpserver"

	// And the built-in in-memory lease database
	_ "github.com/nextdhcp/nextdhcp/core/lease/builtin"
)
