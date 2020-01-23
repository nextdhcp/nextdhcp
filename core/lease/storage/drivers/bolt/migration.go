package bolt

import (
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	schemaVersionBucket = []byte("schema-version")
	schemaVersionKey    = []byte("nextdhcp-schema-version")
)

type migrationFunc func(*bbolt.Tx) error

var migrations = map[string]migrationFunc{
	"0": v0ToV1,
}

func migrateDatabase(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		versionBucket, err := tx.CreateBucketIfNotExists(schemaVersionBucket)
		if err != nil {
			return err
		}

		version := string(versionBucket.Get(schemaVersionKey))
		if version == "" {
			version = "0"
		}

		for version != SchemaVersion {
			migrator, ok := migrations[version]
			if !ok {
				return fmt.Errorf("cannot migrate from %q", version)
			}

			if err := migrator(tx); err != nil {
				return err
			}

			version = string(versionBucket.Get(schemaVersionKey))
		}

		return nil
	})
}
