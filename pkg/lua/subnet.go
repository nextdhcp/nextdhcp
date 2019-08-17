package lua

import (
	"net"
	"sync"

	"github.com/yuin/gluamapper"
	lua "github.com/yuin/gopher-lua"
)

// SubnetConfig holds the configuration for a subnet
type SubnetConfig struct {
	// Database is the lease database to use. Defaults to internal
	Database string `mapstructure:"database"`

	// Ranges of IP address that can be leased to clients on this subnet
	Ranges [][]string `mapstructure:"ranges"`

	// Options holds additional DHCP options for clients on this subnet
	Options map[string]interface{} `mapstructure:"options"`

	// LeaseTime is the default lease time for new IP address leases
	LeaseTime string `mapstructure:"leaseTime"` // TODO(ppacher) use DecodeHook and make time.Duration?

	// Offer is a callback function to modify a lease offer before it is sent
	// to a client
	Offer *lua.LFunction `mapstructure:"offer"`
}

// Subnet defines a subnet to be served
type Subnet struct {
	// IP is the IP address to listen on
	IP net.IP

	// Network is the IP network that is served by this declaration
	Network net.IPNet

	// SubnetConfig embedds additional configuration values
	SubnetConfig
}

type SubnetManager struct {
	rwl     sync.RWMutex
	subnets []Subnet
}

// Setup configures the provided lua VM and exposes subnet related configuration
// functions
func (mng *SubnetManager) Setup(L *lua.LState) error {
	L.SetGlobal("subnet", L.NewFunction(mng.declareSubnet))
	return nil
}

func (mng *SubnetManager) declareSubnet(L *lua.LState) int {
	str := L.ToString(1)
	if str == "" {
		L.ArgError(1, "expected IP CIDR network")
		return 0
	}

	ip, ipNet, err := net.ParseCIDR(str)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	L.Push(L.NewFunction(mng.configureSubnet(ip, *ipNet)))

	return 1
}

func (mng *SubnetManager) configureSubnet(ip net.IP, network net.IPNet) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.ToTable(1)
		if tbl == nil {
			L.ArgError(1, "expected subnet configuration")
			return 0
		}

		var cfg SubnetConfig
		if err := gluamapper.Map(tbl, &cfg); err != nil {
			L.RaiseError(err.Error())
			return 0
		}

		subnet := Subnet{
			IP:           ip,
			Network:      network,
			SubnetConfig: cfg,
		}

		mng.rwl.Lock()
		defer mng.rwl.Unlock()
		mng.subnets = append(mng.subnets, subnet)

		return 0
	}
}
