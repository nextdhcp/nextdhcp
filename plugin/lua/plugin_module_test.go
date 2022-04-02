package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func getPluginTestVM(t *testing.T) (*lua.LState, *PluginManager) {
	l := lua.NewState()

	p := &PluginManager{}
	err := p.Setup(l)

	if err != nil {
		return nil, err
	}

	return l, p
}

func Test_PluginManager_plugin_exists(t *testing.T) {
	vm, m := getPluginTestVM(t)
	assert.NotNil(t, vm)
	assert.NotNil(t, m)

	fn := vm.GetGlobal("plugin")
	assert.NotNil(t, fn)
	assert.Equal(t, lua.LTFunction, fn.Type())
}

func Test_PluginManager_plugin_register_valid(t *testing.T) {
	vm, p := getPluginTestVM(t)

	err := vm.DoString(`
	plugin "test" {
		path = "path/to/test.so",
		some_other_option = "other",
		yet_another_option = 2
	}
	`)
	assert.NoError(t, err)

	assert.Len(t, p.Plugins(), 1)

	assert.Equal(t, PluginConfig{
		Name: "test",
		Path: "path/to/test.so",
		Config: map[string]interface{}{
			"SomeOtherOption":  "other",
			"YetAnotherOption": float64(2),
		},
	}, p.Plugins()[0])
}

func Test_PluginManager_plugin_register_invalid(t *testing.T) {
	vm, _ := getPluginTestVM(t)

	assert.Error(t, vm.DoString(`plugin()`))
	assert.Error(t, vm.DoString(`plugin(nil)`))

	assert.Error(t, vm.DoString(`plugin "name" ()`))
	assert.Error(t, vm.DoString(`plugin "name" ("")`))
	assert.Error(t, vm.DoString(`plugin "name" {1, 2}`))
	assert.Error(t, vm.DoString(`plugin "name" {{} = "test"}`))
	assert.Error(t, vm.DoString(`plugin "name" {}`))
	assert.Error(t, vm.DoString(`plugin "name" {path = 2}`))
}
