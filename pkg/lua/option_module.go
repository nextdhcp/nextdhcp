package lua

import (
	"errors"
	"fmt"
	"sync"

	"github.com/insomniacslk/dhcp/dhcpv4"
	lua "github.com/yuin/gopher-lua"
)

// optionCode implements dhcpv3.OptionCode
type optionCode struct {
	code    uint8
	luaName string
}

func (c *optionCode) Code() uint8 {
	return c.code
}

func (c *optionCode) String() string {
	return fmt.Sprintf("%s (0x%x)", c.luaName, c.Code())
}

// compile time test
var _ dhcpv4.OptionCode = &optionCode{}

// OptionModule keeps track of well-known and user-defined DHCPv4 options and
// how to covert them between their DHCPv4 wire and lua representation.
type OptionModule struct {
	l          sync.RWMutex
	nameToCode map[string]dhcpv4.OptionCode     // access protected by l
	codeToType map[dhcpv4.OptionCode]*KnownType // access protected by l
}

// NewOptionModule creates a new option module with support for the named DHCP options configured
// in names. Each entry must have a dedicated KnownType entry in the types map. Any type registered
// by the lua rc file will be added to those maps so access to them should by synchronized with
// the options module
func NewOptionModule(names map[string]dhcpv4.OptionCode, types map[dhcpv4.OptionCode]*KnownType) *OptionModule {
	return &OptionModule{
		nameToCode: names,
		codeToType: types,
	}
}

// DeclareOption declares a new DHCPv4 option
func (opts *OptionModule) DeclareOption(name string, code uint8, typeName string) error {
	opts.l.Lock()
	defer opts.l.Unlock()

	if _, ok := opts.nameToCode[name]; ok {
		return errors.New("option code already registred")
	}

	knownType, ok := typeKeyToType[typeName]
	if !ok {
		return errors.New("unsupport type")
	}

	opt := &optionCode{code, name}

	opts.codeToType[opt] = knownType
	opts.nameToCode[name] = opt

	return nil
}

// TypeForName returns the a KnownType definition for the given name
func (opts *OptionModule) TypeForName(name string) (*KnownType, dhcpv4.OptionCode, bool) {
	opts.l.RLock()
	defer opts.l.RUnlock()

	code, ok := opts.nameToCode[name]
	if !ok {
		return nil, nil, false
	}

	knownType, ok := opts.codeToType[code]
	return knownType, code, ok
}

// Setup configures the lua state L and adds global symbols to interact with
// the options module
func (opts *OptionModule) Setup(L *lua.LState) error {
	for key := range typeKeyToType {
		L.SetGlobal(key, lua.LString(key))
	}

	// declare_option("architecture", 0x99, TYPE_UINT16)
	L.SetGlobal("declare_option", L.NewFunction(opts.luaDeclareOption))

	return nil
}

func (opts *OptionModule) luaDeclareOption(L *lua.LState) int {
	name, ok := L.Get(1).(lua.LString)
	if !ok {
		L.ArgError(1, "exptected a string name")
		return 0
	}

	b := L.ToInt(2)
	// TODO(ppacher): we actually need a better type check here
	if b < 1 || b > 255 {
		L.ArgError(2, "expected a number between 1 and 255 (0x01 and 0xff)")
		return 0
	}

	typeName, ok := L.Get(3).(lua.LString)
	if !ok {
		L.ArgError(3, "expected a type")
		return 0
	}

	if err := opts.DeclareOption(string(name), uint8(b), string(typeName)); err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	return 0
}
