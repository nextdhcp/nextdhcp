package core

import (
	// Include all built-in directives
	_ "github.com/nextdhcp/nextdhcp/plugin/database"
	_ "github.com/nextdhcp/nextdhcp/plugin/gotify"
	_ "github.com/nextdhcp/nextdhcp/plugin/ifname"
	_ "github.com/nextdhcp/nextdhcp/plugin/lease"
	_ "github.com/nextdhcp/nextdhcp/plugin/log"
	_ "github.com/nextdhcp/nextdhcp/plugin/mqtt"
	_ "github.com/nextdhcp/nextdhcp/plugin/nextserver"
	_ "github.com/nextdhcp/nextdhcp/plugin/option"
	_ "github.com/nextdhcp/nextdhcp/plugin/prometheus"
	_ "github.com/nextdhcp/nextdhcp/plugin/ranges"
	_ "github.com/nextdhcp/nextdhcp/plugin/servername"
	_ "github.com/nextdhcp/nextdhcp/plugin/static"
)
