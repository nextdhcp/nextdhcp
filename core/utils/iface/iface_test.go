package iface

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByIP(t *testing.T) {
	iface, err := ByIP(net.IP{127, 0, 0, 1})
	assert.NoError(t, err)
	assert.Equal(t, "lo", iface.Name)

	iface, err = ByIP(net.IP{127, 0, 1, 1})
	assert.Error(t, err)
	assert.Nil(t, iface)
}

func TestContains(t *testing.T) {
	iface, inet, err := Contains(net.IP{127, 0, 0, 1})
	assert.NoError(t, err)
	assert.Equal(t, "lo", iface.Name)
	assert.NotNil(t, inet)
	assert.Equal(t, "127.0.0.1/8", inet.String())

	iface, inet, err = Contains(net.IP{127, 0, 1, 1})
	assert.NoError(t, err)
	assert.Equal(t, "lo", iface.Name)
	assert.NotNil(t, inet)
	assert.Equal(t, "127.0.0.1/8", inet.String())

	iface, inet, err = Contains(net.IP{1, 2, 3, 4})
	assert.Error(t, err)
	assert.Nil(t, iface)
	assert.Nil(t, inet)
}

func TestByNameOrCIDR(t *testing.T) {
	ip, inet, err := ByNameOrCIDR("lo")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", ip.String())
	assert.Equal(t, "127.0.0.1/8", inet.String())

	ip, inet, err = ByNameOrCIDR("127.0.0.1/8")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", ip.String())
	assert.Equal(t, "127.0.0.0/8", inet.String())

	ip, inet, err = ByNameOrCIDR("notAnIpOrInterface")
	require.Error(t, err)
}
