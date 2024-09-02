package bolt

import (
	"context"
	"os"
	"testing"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/core/lease/storage/tests"
	"go.etcd.io/bbolt"
)

func TestMemoryStorage(t *testing.T) {
	factory := func(ctx context.Context) storage.LeaseStorage {
		file, err := os.CreateTemp("", "storage-*.db")
		if err != nil {
			panic(err.Error())
		}

		db, err := bbolt.Open(file.Name(), 0o600, &bbolt.Options{
			OpenFile: func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				return file, nil
			},
		})
		if err != nil {
			panic(err.Error())
		}

		if err := migrateDatabase(db); err != nil {
			panic(err)
		}

		return &Storage{
			db:   db,
			path: file.Name(),
		}
	}

	teardown := func(s storage.LeaseStorage) {
		db := s.(*Storage)
		db.db.Close()
		os.Remove(db.path)
	}

	tests.Run(t, factory, teardown)
}
