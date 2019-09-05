package bolt

import (
	"fmt"

	"github.com/etcd-io/bbolt"
	"github.com/nextdhcp/nextdhcp/core/lease"
)

func init() {
	lease.MustRegisterDriver("bolt", Setup)
}

// Setup implements lease.Factory and opens a new bolt database
func Setup(arguments map[string][]string) (lease.Database, error) {
	file := ""

	if args, ok := arguments["__args__"]; ok {
		file = args[0]
	} else if f, ok := arguments["file"]; ok {
		if len(f) > 1 {
			return nil, fmt.Errorf("only one database file can be configured")
		}

		file = f[0]
	} else {
		return nil, fmt.Errorf("no database file configured")
	}

	db, err := bbolt.Open(file, 0660, nil)
	if err != nil {
		return nil, err
	}

	d := &Database{DB: db}
	if err := d.Setup(); err != nil {
		return nil, err
	}

	return d, nil
}
