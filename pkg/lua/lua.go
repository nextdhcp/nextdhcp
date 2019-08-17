package lua

import (
	"io"
	"io/ioutil"
	"os"

	lua "github.com/yuin/gopher-lua"
)

// Runner is cool :)
// TODO(ppacher): fix comment
type Runner struct {
	vm      *lua.LState
	plugins *PluginManager
	subnets *SubnetManager
}

// NewFromReader creates and returns a new lua runner from the given input
// reader
func NewFromReader(input io.Reader) (*Runner, error) {
	content, err := ioutil.ReadAll(input)
	if err != nil {
		return nil, err
	}

	r := &Runner{
		vm:      lua.NewState(),
		plugins: &PluginManager{},
		subnets: &SubnetManager{},
	}

	if err := r.plugins.Setup(r.vm); err != nil {
		return nil, err
	}
	if err := r.subnets.Setup(r.vm); err != nil {
		return nil, err
	}

	if err := r.vm.DoString(string(content)); err != nil {
		return nil, err
	}

	return r, nil
}

// NewFromFile creates and returns a new Runner from the given input
// configuration file
func NewFromFile(filepath string) (*Runner, error) {
	reader, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return NewFromReader(reader)
}

// Plugins returns all registred plugins
func (r *Runner) Plugins() []PluginConfig {
	return r.plugins.Plugins()
}

// Subnets returns all registered subnets
func (r *Runner) Subnets() []Subnet {
	return r.subnets.Subnets()
}
