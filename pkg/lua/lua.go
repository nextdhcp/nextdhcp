package lua

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	"github.com/ppacher/dhcp-ng/pkg/middleware"
	"github.com/ppacher/glua-loop/pkg/eventloop"
	lua "github.com/yuin/gopher-lua"
)

// Runner is cool :)
// TODO(ppacher): fix comment
type Runner struct {
	loop    eventloop.Loop
	plugins *PluginManager
	subnets *SubnetManager
	options *OptionModule
}

// NewFromReader creates and returns a new lua runner from the given input
// reader
func NewFromReader(input io.Reader) (*Runner, error) {
	content, err := ioutil.ReadAll(input)
	if err != nil {
		return nil, err
	}

	r := &Runner{
		plugins: &PluginManager{},
		subnets: &SubnetManager{},
		options: NewOptionModule(optionNames, optionTypes),
	}

	opts := &eventloop.Options{
		InitVM: func(L *lua.LState) error {
			if err := r.plugins.Setup(L); err != nil {
				return err
			}
			if err := r.subnets.Setup(L); err != nil {
				return err
			}
			if err := r.options.Setup(L); err != nil {
				return err
			}

			return nil
		},
	}

	loop, err := eventloop.New(opts)
	if err != nil {
		return nil, err
	}
	r.loop = loop

	if err := r.loop.Start(context.Background()); err != nil {
		return nil, err
	}

	r.loop.ScheduleAndWait(func(L *lua.LState) {
		err = L.DoString(string(content))
	})

	if err != nil {
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

// FunctionHandler returns a middleware.Handler that executes a lua function
// on the runner
func (r *Runner) FunctionHandler(fn *lua.LFunction) middleware.Handler {
	return &luaMiddlware{
		runner: r,
		fn:     fn,
	}
}
