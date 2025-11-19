package lua

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	lua "github.com/yuin/gopher-lua"
)

// prepareOptionTable sets a metatable to tbl with a newindex
// field that parses values for DHCP options
func prepareOptionTable(L *lua.LState, tbl *lua.LTable, rcv map[dhcpv4.OptionCode]dhcpv4.OptionValue, opts *OptionModule) error {
	mt := L.NewTable()

	mt.RawSetString("__newindex", L.NewFunction(func(L *lua.LState) int {
		t := L.ToTable(1)
		k := L.ToString(2)
		v := L.Get(3)

		if t == nil {
			L.ArgError(1, "We should get a table in __newindex")
			return 0
		}

		if k == "" {
			L.ArgError(2, "option table requires string keys")
			return 0
		}

		fn, opCode, ok := opts.TypeForName(k)
		if !ok {
			L.ArgError(2, "invalid option name")
		}

		//
		if v == lua.LNil {
			delete(rcv, opCode)
			return 0
		}

		value, err := fn.FromLuaValue(L, v)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}

		rcv[opCode] = value

		return 0
	}))

	L.SetMetatable(tbl, mt)

	return nil
}
