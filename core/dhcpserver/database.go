package dhcpserver

import "github.com/nextdhcp/nextdhcp/core/lease"

func openDatabase(c *Config) error {
	// If the database is already opened we can bail out
	if c.Database != nil {
		return nil
	}

	db, err := lease.Open("", map[string][]string{})
	if err != nil {
		return err
	}

	c.Database = db

	return nil
}
