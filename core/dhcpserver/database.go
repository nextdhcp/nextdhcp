package dhcpserver

import "github.com/nextdhcp/nextdhcp/core/lease/storage"

// DatabaseConfig selects and configures the lease database of a server.
type DatabaseConfig struct {
	// Driver is the name of the storage driver, for example "bolt" or
	// "memory".
	Driver string `json:"driver,omitempty"`

	// Options are passed to the driver. The arguments that follow the
	// driver name in the Dhcpfile are stored under the key "__args__".
	Options map[string][]string `json:"options,omitempty"`
}

func (s *Server) openDatabase() error {
	driver := "bolt"
	options := map[string][]string{
		"file": {s.IP.String() + ".db"},
	}

	if s.DatabaseConfig != nil {
		driver = s.DatabaseConfig.Driver
		options = s.DatabaseConfig.Options
	}

	store, err := storage.Open(driver, options)
	if err != nil {
		return err
	}

	db := storage.NewDatabase(store)
	db.SetLogger(s.logger)

	s.store = store
	s.Database = db

	return nil
}
