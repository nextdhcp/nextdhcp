package dhcpserver

import "github.com/nextdhcp/nextdhcp/core/lease"

func openDatabase(c *Config) error {
	// TODO(ppacher): rework the database handling part
	// to use a more Caddyfile like setup
	db, err := lease.Open("", map[string]interface{}{
		"network": c.Network,
	})
	if err != nil {
		return err
	}

	c.Database = db

	return nil
}
