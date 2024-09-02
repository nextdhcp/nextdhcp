package bolt

import (
	"fmt"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"go.etcd.io/bbolt"
)

func init() {
	storage.MustRegister("bolt", storageFactory)
}

func storageFactory(arguments map[string][]string) (storage.LeaseStorage, error) {
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

	db, err := bbolt.Open(file, 0o660, nil)
	if err != nil {
		return nil, err
	}

	d := &Storage{db: db}

	return d, nil
}
