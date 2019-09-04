package core

import (
	// Include all built-in directives
	_ "github.com/nextdhcp/nextdhcp/plugin/ifname"
	_ "github.com/nextdhcp/nextdhcp/plugin/lease"
	_ "github.com/nextdhcp/nextdhcp/plugin/nextserver"
	_ "github.com/nextdhcp/nextdhcp/plugin/option"
	_ "github.com/nextdhcp/nextdhcp/plugin/ranges"
	_ "github.com/nextdhcp/nextdhcp/plugin/servername"
)
