package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func getPluginTestVM(t *testing.T) (*lua.LState, *PluginManager) {
	l := lua.NewState()

	p := &PluginManager{}
	p.Setup(l)

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

	assert.Len(t, p.plugins, 1)

	assert.Equal(t, PluginConfig{
		Name: "test",
		Path: "path/to/test.so",
		Config: map[string]interface{}{
			"SomeOtherOption":  "other",
			"YetAnotherOption": float64(2),
		},
	}, p.plugins[0])
}
