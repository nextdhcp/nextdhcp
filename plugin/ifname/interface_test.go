package ifname

import (
	"testing"

	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
)

func TestInterfacePlugin(t *testing.T) {
	t.Run("valid interface", func(t *testing.T) {
		c := test.CreateTestBed(t, "interface lo")

		// reset the interface so we can check if setupInterface
		// configures it correctly
		cfg := c.Server
		cfg.Interface = ""

		assert.NoError(t, setupInterface(c))
		assert.Equal(t, "lo", cfg.Interface)
	})

	t.Run("invalid interface", func(t *testing.T) {
		c := test.CreateTestBed(t, "interface someInterfaceThatDoesNotExist")
		assert.Error(t, setupInterface(c))
	})

	t.Run("no interface name", func(t *testing.T) {
		c := test.CreateTestBed(t, "interface")
		assert.Error(t, setupInterface(c))
	})
}
