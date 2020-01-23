package core

import (
	// Plugin the dhcpserver
	_ "github.com/nextdhcp/nextdhcp/core/dhcpserver"

	// And the built-in in-memory lease database
	// as well as the bolddb based one
	_ "github.com/nextdhcp/nextdhcp/core/lease/storage/drivers"
)
