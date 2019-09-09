package dhcpserver

import "github.com/nextdhcp/nextdhcp/core/lease"

func ensureDatabase(c *Config) error {
	// If the database is already opened we can bail out
	if c.Database != nil {
		return nil
	}

	db, err := lease.Open("bolt", map[string][]string{
		"file": []string{c.IP.String() + ".db"},
	})
	if err != nil {
		return err
	}

	c.Database = db

	return nil
}
