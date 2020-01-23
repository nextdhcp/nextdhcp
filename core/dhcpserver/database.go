package dhcpserver

import "github.com/nextdhcp/nextdhcp/core/lease/storage"

func ensureDatabase(c *Config) error {
	// If the database is already opened we can bail out
	if c.Database != nil {
		return nil
	}

	db, err := storage.Open("bolt", map[string][]string{
		"file": {c.IP.String() + ".db"},
	})
	if err != nil {
		return err
	}

	c.Database = storage.NewDatabase(db)

	return nil
}
