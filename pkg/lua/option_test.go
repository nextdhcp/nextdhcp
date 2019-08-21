package lua

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func Test_WellKnownTypes_Factory_FromLua(t *testing.T) {
	vm := lua.NewState()

	t.Run("TYPE_IP", func(t *testing.T) {
		value := lua.LString("10.1.0.1")
		opt, err := TypeIP.FromLuaValue(vm, value)
		assert.Nil(t, err)
		assert.Equal(t, "10.1.0.1", opt.String())

		strRep, err := TypeIP.FromValue(vm, opt)
		assert.NoError(t, err)
		assert.Equal(t, strRep, lua.LString("10.1.0.1"))

		testStr := func(s string) error {
			_, err := TypeIP.FromLuaValue(vm, lua.LString(s))
			return err
		}

		assert.Error(t, testStr(""))
		assert.Error(t, testStr("10.1000.1.1"))

		_, err = TypeIP.FromLuaValue(vm, lua.LTrue)
		assert.Error(t, err)

		_, err = TypeIP.FromLuaValue(vm, lua.LNil)
		assert.Error(t, err)
	})

	t.Run("TYPE_IP_LIST", func(t *testing.T) {
		tbl := vm.NewTable()

		tbl.Append(lua.LString("10.1.0.1"))
		tbl.Append(lua.LString("10.1.0.100"))

		opt, err := TypeIPList.FromLuaValue(vm, tbl)
		assert.Nil(t, err)

		luaRep, err := TypeIPList.FromValue(vm, opt)
		assert.NoError(t, err)
		assert.Equal(t, tbl, luaRep)

		ips, ok := opt.(dhcpv4.IPs)
		assert.True(t, ok)
		assert.Len(t, []net.IP(ips), 2)

		_, err = TypeIPList.FromLuaValue(vm, lua.LTrue)
		assert.Error(t, err)

		_, err = TypeIPList.FromLuaValue(vm, lua.LNil)
		assert.Error(t, err)
	})
}

func Test_OptionTable_setter(t *testing.T) {
	vm := lua.NewState()
	tbl := vm.NewTable()
	rcv := make(map[dhcpv4.OptionCode]dhcpv4.OptionValue)

	assert.NoError(t, prepareOptionTable(vm, tbl, rcv))
	vm.SetGlobal("options", tbl)

	err := vm.DoString(`options.netmask = "255.255.242.0"`)

	assert.NoError(t, err)
	assert.NotNil(t, rcv[dhcpv4.OptionSubnetMask])
	assert.Equal(t, "255.255.242.0", rcv[dhcpv4.OptionSubnetMask].String())

	// nil should delete it
	assert.NoError(t, vm.DoString(`options.netmask = nil`))
	assert.Nil(t, rcv[dhcpv4.OptionSubnetMask])

	// test invalid types or parse errors
	assert.Error(t, vm.DoString(`options.netmask = 1`))
	assert.Error(t, vm.DoString(`options.netmask = ""`))
	assert.Error(t, vm.DoString(`options.netmask = "10.10.10.1000"`))

	assert.Error(t, vm.DoString(`options.unknown = ""`))

	// test list of IPs
	assert.NoError(t, vm.DoString(`options.router = {
		"10.0.0.1",
		"10.0.0.2",
	}`))
	assert.NotNil(t, rcv[dhcpv4.OptionRouter])
	assert.Len(t, rcv[dhcpv4.OptionRouter], 2)
}
