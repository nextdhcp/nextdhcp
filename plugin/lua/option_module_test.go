package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func getOptionModule(t *testing.T) *OptionModule {
	names := GetBuiltinOptionNames()
	types := GetBuiltinOptionTypes()

	opts := NewOptionModule(names, types)
	if !assert.NotNil(t, opts) {
		t.FailNow()
	}

	return opts
}

func getOptionTestVM(t *testing.T) (*lua.LState, *OptionModule) {
	l := lua.NewState()

	opts := getOptionModule(t)
	err := opts.Setup(l)
	assert.NoError(t, err)

	return l, opts
}

func Test_OptionModule(t *testing.T) {
	opts := getOptionModule(t)

	t.Run("declaring options", func(t *testing.T) {
		t.Run("should work if new", func(t *testing.T) {
			assert.NoError(t, opts.DeclareOption("arch", 0x98, "TYPE_STRING"))
			assert.NotNil(t, opts.nameToCode["arch"])
			assert.Equal(t, "arch (0x98)", opts.nameToCode["arch"].String())
			assert.NotNil(t, opts.codeToType[opts.nameToCode["arch"]])

			k, _, ok := opts.TypeForName("arch")
			assert.NotNil(t, k)
			assert.True(t, ok)
			assert.Equal(t, TypeString, k)
		})

		t.Run("should fail if name is used", func(t *testing.T) {
			assert.Error(t, opts.DeclareOption("arch", 0xaa, "TYPE_IP"))
			assert.Equal(t, TypeString, opts.codeToType[opts.nameToCode["arch"]])
		})

		t.Run("should fail for unknown types", func(t *testing.T) {
			assert.Error(t, opts.DeclareOption("foo", 0x00, "TYPE_SOMETHING"))
			k, _, ok := opts.TypeForName("foo")
			assert.False(t, ok)
			assert.Nil(t, k)
		})
	})
}

func Test_OptionModule_Lua(t *testing.T) {
	vm, opts := getOptionTestVM(t)

	assert.NoError(t, vm.DoString(`declare_option("arch", 0x99, TYPE_STRING)`))
	arch, _, ok := opts.TypeForName("arch")
	assert.NotNil(t, arch)
	assert.Equal(t, TypeString, arch)
	assert.True(t, ok)

	assert.Error(t, vm.DoString(`declare_option(1, 0x99, TYPE_STRING)`))
	assert.Error(t, vm.DoString(`declare_option(nil, 0x99, TYPE_STRING)`))
	assert.Error(t, vm.DoString(`declare_option("test", 0.1, TYPE_STRING)`))
	assert.Error(t, vm.DoString(`declare_option("test", -100, TYPE_STRING)`))
	assert.Error(t, vm.DoString(`declare_option("test", 1000, TYPE_STRING)`))
	assert.Error(t, vm.DoString(`declare_option("test", 0x10, "foobar)`))
	assert.Error(t, vm.DoString(`declare_option("test", 0x10, 1)`))
	assert.Error(t, vm.DoString(`declare_option("test", 0x10, nil)`))
}
