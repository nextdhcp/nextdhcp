package memory

import (
	"context"
	"testing"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/core/lease/storage/tests"
)

func TestMemoryStorage(t *testing.T) {
	factory := func(ctx context.Context) storage.LeaseStorage {
		return makeStorage()
	}

	teardown := func(_ storage.LeaseStorage) {}

	tests.Run(t, factory, teardown)
}
