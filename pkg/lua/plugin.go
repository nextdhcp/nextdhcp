package lua

import (
	"sync"

	"github.com/yuin/gluamapper"
	lua "github.com/yuin/gopher-lua"
)

type PluginConfig struct {
	// Name is the name of the plugin
	Name string

	// Path holds the file path to the plugins .so file
	Path string

	// Config holds additional configuration values for the plugin
	Config map[string]interface{}
}

// PluginManager allows configuration of loadable
type PluginManager struct {
	rwl     sync.RWMutex
	plugins []PluginConfig
}

// Setup initializes the given lua.LState by configuring global plugin related functions
func (mng *PluginManager) Setup(L *lua.LState) error {
	L.SetGlobal("plugin", L.NewFunction(mng.declarePlugin))
	return nil
}

func (mng *PluginManager) declarePlugin(L *lua.LState) int {
	name := L.ToString(1)
	if name == "" {
		L.ArgError(1, "expected plugin name")
		return 0
	}

	L.Push(L.NewFunction(mng.configurePlugin(name)))

	return 1
}

func (mng *PluginManager) configurePlugin(name string) lua.LGFunction {
	return func(L *lua.LState) int {
		param := L.Get(1)
		if param == nil {
			L.ArgError(1, "expected configuration table but got nil")
			return 0
		}

		if param.Type() != lua.LTTable {
			L.ArgError(1, "expected configuration table but got something else")
			return 0
		}

		// TODO(ppacher): we should likely use a different option set here
		opt := gluamapper.NewMapper(gluamapper.Option{}).Option

		configMapInterface := gluamapper.ToGoValue(param, opt)
		if configMapInterface == nil {
			L.ArgError(1, "expected configuration table but got something else")
			return 0
		}

		configMap, ok := configMapInterface.(map[interface{}]interface{})
		if !ok {
			L.ArgError(1, "expected configuration table but got something else")
			return 0
		}

		converted := make(map[string]interface{}, len(configMap))
		// plugin configuration must be map[string]interface{} so we need to convert it
		for keyInterface, value := range configMap {
			key, ok := keyInterface.(string)
			if !ok {
				L.ArgError(1, "configuration table must have string keys")
				return 0
			}

			converted[key] = value
		}

		pathInterface, ok := converted["path"]
		if !ok {
			pathInterface, ok = converted["Path"]
			delete(converted, "Path")
		}
		delete(converted, "path")

		if !ok {
			L.ArgError(1, "configuration table must include a string `path` key")
			return 0
		}

		path, ok := pathInterface.(string)
		if !ok {
			L.ArgError(1, "configuration table must include a string `path` key")
			return 0
		}

		mng.rwl.Lock()
		defer mng.rwl.Unlock()

		mng.plugins = append(mng.plugins, PluginConfig{
			Name:   name,
			Path:   path,
			Config: converted,
		})

		return 0
	}
}
