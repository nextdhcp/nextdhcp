package lease

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterDriver(t *testing.T) {
	factory := func(_ map[string][]string) (Database, error) {
		return nil, errors.New("dummy factory stub")
	}

	assert.NoError(t, RegisterDriver("test", factory))
	assert.NotNil(t, drivers["test"])

	assert.Error(t, RegisterDriver("test", nil))
	delete(drivers, "test")

	assert.NotPanics(t, func() {
		MustRegisterDriver("test", factory)
	})

	assert.Panics(t, func() {
		MustRegisterDriver("test", factory)
	})
	delete(drivers, "test")
}

func TestOpenDriver(t *testing.T) {
	var expectedOpts map[string][]string
	called := false

	factory := func(opts map[string][]string) (Database, error) {
		assert.Equal(t, expectedOpts, opts)
		called = true

		return nil, fmt.Errorf("error stub")
	}

	MustRegisterDriver("test-driver", factory)

	db, err := Open("non-existent", nil)
	assert.Nil(t, db)
	assert.Error(t, err)
	assert.False(t, called)

	expectedOpts = map[string][]string{
		"opt1": []string{"v1", "v2"},
	}
	db, err = Open("test-driver", expectedOpts)
	assert.Nil(t, db)
	require.Error(t, err)
	assert.Equal(t, "error stub", err.Error())
	assert.True(t, called)

	assert.Panics(t, func() {
		MustOpen("not-existent", nil)
	})

	assert.Panics(t, func() {
		MustOpen("test-driver", expectedOpts)
	})
}
