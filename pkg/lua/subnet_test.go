package lua

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func getTestVM(t *testing.T) (*lua.LState, *SubnetManager) {
	l := lua.NewState()

	m := &SubnetManager{}
	m.Setup(l)

	return l, m
}

func Test_SubnetManager_subnet_exists(t *testing.T) {
	vm, m := getTestVM(t)
	assert.NotNil(t, vm)
	assert.NotNil(t, m)

	fn := vm.GetGlobal("subnet")
	assert.NotNil(t, fn)
	assert.Equal(t, lua.LTFunction, fn.Type())
}

func Test_SubnetManager_subnet_register_valid(t *testing.T) {
	vm, m := getTestVM(t)

	err := vm.DoString(`
	subnet "10.1.0.1/24" {
		database = "etcd",
		ranges = {
			{"10.1.0.100", "10.1.0.200"}
		},
		options = {
			routers = { "10.1.0.1" },
		},
		leaseTime = "10m",
		offer = function() end
	}
	`)

	assert.NoError(t, err)

	assert.Len(t, m.subnets, 1)
	assert.NotNil(t, m.subnets[0].Offer)

	// we cannot check equality on functions so
	m.subnets[0].Offer = nil

	assert.Equal(t, Subnet{
		IP: net.IP{10, 1, 0, 1}.To16(),
		Network: net.IPNet{
			IP:   net.IP{10, 1, 0, 0}.To4(),
			Mask: net.IPMask{255, 255, 255, 0},
		},
		SubnetConfig: SubnetConfig{
			Database: "etcd",
			Ranges: [][]string{
				{"10.1.0.100", "10.1.0.200"},
			},
			Options: map[string]interface{}{
				"Routers": []interface{}{"10.1.0.1"},
			},
			LeaseTime: "10m",
		},
	}, m.subnets[0])
}
