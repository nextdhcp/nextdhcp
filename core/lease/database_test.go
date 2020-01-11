package lease

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseContextKeys(t *testing.T) {
	ctx := context.Background()

	// invalid types should panic
	ctx = context.WithValue(ctx, Key{}, "invalid-type")
	assert.Panics(t, func() {
		GetDatabase(ctx)
	})

	db := Database(nil)
	ctx = WithDatabase(ctx, db)
	assert.Equal(t, db, GetDatabase(ctx))
}
