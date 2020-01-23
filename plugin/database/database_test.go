package database

import (
	"math/rand"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
)

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

	storage.Register(driverName, func(opts map[string][]string) (storage.LeaseStorage, error) {
		argsOpts = opts
		return ret, retErr
	})

	t.Run("no args", func(t *testing.T) {
		c := test.CreateTestBed(t, "database test-driver")
		assert.NoError(t, parseDatabaseDirective(c))
		assert.NotNil(t, dhcpserver.GetConfig(c).Database)
	})

	t.Run("args", func(t *testing.T) {
		c := test.CreateTestBed(t, `database test-driver some arguments {
			barg1 1
			barg2 2 3
		}`)
		assert.NoError(t, parseDatabaseDirective(c))
		assert.NotNil(t, dhcpserver.GetConfig(c).Database)
		assert.NotNil(t, argsOpts)

		expected := map[string][]string{
			"__args__": []string{"some", "arguments"},
			"barg1":    []string{"1"},
			"barg2":    []string{"2", "3"},
		}
		assert.Equal(t, expected, argsOpts)
	})

	t.Run("invalid", func(t *testing.T) {
		c := test.CreateTestBed(t, "database")
		assert.Error(t, parseDatabaseDirective(c))

		c = test.CreateTestBed(t, "")
		assert.Error(t, parseDatabaseDirective(c))

		c = test.CreateTestBed(t, "database invalid-driver")
		assert.Error(t, parseDatabaseDirective(c))

		c = test.CreateTestBed(t, `database test-driver {
			block arg
		} something else`)
		assert.Error(t, parseDatabaseDirective(c))
	})
}
