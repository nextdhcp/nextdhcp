package option

import (
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKnown(t *testing.T) {
	code, value, err := ParseKnown("hostname", []string{"example"})
	require.NoError(t, err)
	assert.Equal(t, dhcpv4.OptionHostName, code)
	assert.Equal(t, []byte("example"), value.ToBytes())

	code, value, err = ParseKnown("router", []string{"10.0.0.1", "10.0.0.2"})
	require.NoError(t, err)
	assert.Equal(t, dhcpv4.OptionRouter, code)
	assert.Equal(t, []byte{10, 0, 0, 1, 10, 0, 0, 2}, value.ToBytes())

	_, _, err = ParseKnown("no-such-option", []string{"value"})
	assert.Equal(t, ErrUnknownOption, err)
}

// TestParseKnownSingleValue checks that options with a single value fail
// without exactly one value, instead of panicking without any.
func TestParseKnownSingleValue(t *testing.T) {
	for _, values := range [][]string{nil, {}, {"one", "two"}} {
		assert.NotPanics(t, func() {
			_, _, err := ParseKnown("hostname", values)
			assert.Error(t, err, "%q", values)
		}, "%q", values)
	}
}
