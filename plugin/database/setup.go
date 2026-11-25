package database

import (
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
)

func init() {
	dhcpserver.RegisterDirective("database", parseDatabaseDirective)
}

func parseDatabaseDirective(c dhcpserver.Helper) error {
	if !c.Next() {
		return c.ArgErr()
	}

	if !c.NextArg() {
		return c.ArgErr()
	}
	driverName := c.Val()

	options := make(map[string][]string)
	remaining := c.RemainingArgs()
	if len(remaining) > 0 {
		options["__args__"] = remaining
	}

	for c.NextBlock(0) {
		options[c.Val()] = c.RemainingArgs()
	}

	// A closing brace that is followed by more tokens on its line doesn't
	// close the block, so the block is still open
	if c.Nesting() > 0 || c.Next() {
		return c.ArgErr()
	}

	// The database is opened when the server is provisioned; only check
	// that the driver exists
	if !storage.IsRegistered(driverName) {
		return c.Errf("unknown database driver %q", driverName)
	}

	c.Server.DatabaseConfig = &dhcpserver.DatabaseConfig{
		Driver:  driverName,
		Options: options,
	}

	return nil
}
