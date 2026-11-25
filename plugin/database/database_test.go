package database

import (
	"math/rand"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDatabase sets up the database directive like v1 did: it also opens
// the database, which now happens when the server is provisioned.
func setupDatabase(t *testing.T, c dhcpserver.Helper) (*dhcpserver.Server, error) {
	if err := parseDatabaseDirective(c); err != nil {
		return nil, err
	}

	srv, err := test.ProvisionServer(t, c.Server)
	require.NoError(t, err)

	return srv, nil
}

var seededRand *rand.Rand

func init() {
	source := rand.NewSource(time.Now().UnixNano())
	seededRand = rand.New(source)
}

func TestDatabaseSetup(t *testing.T) {
	driverName := "test-driver"

	var ret storage.LeaseStorage
	var retErr error
	var argsOpts map[string][]string

	err := storage.Register(driverName, func(opts map[string][]string) (storage.LeaseStorage, error) {
		argsOpts = opts
		return ret, retErr
	})

	assert.NoError(t, err)

	t.Run("no args", func(t *testing.T) {
		c := test.CreateTestBed(t, "database test-driver")
		srv, err := setupDatabase(t, c)
		assert.NoError(t, err)
		assert.NotNil(t, srv.Database)
	})

	t.Run("args", func(t *testing.T) {
		c := test.CreateTestBed(t, `database test-driver some arguments {
			barg1 1
			barg2 2 3
		}`)
		srv, err := setupDatabase(t, c)
		assert.NoError(t, err)
		assert.NotNil(t, srv.Database)
		assert.NotNil(t, argsOpts)

		expected := map[string][]string{
			"__args__": {"some", "arguments"},
			"barg1":    {"1"},
			"barg2":    {"2", "3"},
		}
		assert.Equal(t, expected, argsOpts)
	})

	t.Run("invalid", func(t *testing.T) {
		c := test.CreateTestBed(t, "database")
		_, err := setupDatabase(t, c)
		assert.Error(t, err)

		c = test.CreateTestBed(t, "")
		_, err = setupDatabase(t, c)
		assert.Error(t, err)

		c = test.CreateTestBed(t, "database invalid-driver")
		_, err = setupDatabase(t, c)
		assert.Error(t, err)

		c = test.CreateTestBed(t, `database test-driver {
			block arg
		} something else`)
		_, err = setupDatabase(t, c)
		assert.Error(t, err)
	})
}
