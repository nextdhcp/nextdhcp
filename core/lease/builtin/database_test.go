package builtin

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/nextdhcp/nextdhcp/core/lease/iprange"
	"github.com/stretchr/testify/assert"
)

var (
	ctx           = context.Background()
	defaultClient = &lease.Client{
		HwAddr:   net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		Hostname: "",
	}
	defaultClientMAC = "00:AA:BB:CC:DD:EE"
)

func addReservedIP(db *database, ip string, mac string) uint32 {
	i := net.ParseIP(ip)
	if i == nil {
		panic("Invalid IP")
	}

	m, err := net.ParseMAC(mac)
	if err != nil {
		panic(err)
	}
	key, _ := iprange.IP2Int(i.To4())
	db.reservedAddresses[key] = lease.ReservedAddress{
		Client: lease.Client{HwAddr: m},
		IP:     i.To4(),
	}
	db.reservedAddressesByClient[m.String()] = key

	return key
}

func addLeasedIP(db *database, ip string, mac string) uint32 {
	i := net.ParseIP(ip)
	if i == nil {
		panic("Invalid IP")
	}

	m, err := net.ParseMAC(mac)
	if err != nil {
		panic(err)
	}
	key, _ := iprange.IP2Int(i.To4())
	db.leasedAddresses[key] = &lease.Lease{
		Client:  lease.Client{HwAddr: m},
		Address: i.To4(),
		Expires: time.Now().Add(time.Minute),
	}
	db.leasedAddressesByClient[m.String()] = key

	return key
}

func getTestDatabase(t *testing.T) *database {
	return New().(*database)
}

func Test_Database_Leases(t *testing.T) {
	db := getTestDatabase(t)
	addLeasedIP(db, "192.168.0.10", defaultClientMAC)
	addLeasedIP(db, "192.168.0.11", defaultClientMAC)

	assert.Len(t, db.leasedAddresses, 2)

	leases, err := db.Leases(ctx)
	assert.Nil(t, err)
	assert.Len(t, leases, 2)

	has10 := false
	has11 := false
	idx10 := -1

	for i, l := range leases {
		switch l.Address.String() {
		case "192.168.0.10":
			idx10 = i
			has10 = true
		case "192.168.0.11":
			has11 = true
		default:
			t.Fail()
		}

		assert.Equal(t, l.Client, *defaultClient)
	}

	assert.NotEqual(t, -1, idx10)

	assert.True(t, has10)
	assert.True(t, has11)

	// Leases should return a deep clone of the lease
	key, _ := iprange.IP2Int(net.IP{192, 168, 0, 10})

	// change the first byte of net.IP
	db.leasedAddresses[key].Address[0] = 100
	// now the must not match anymore
	assert.Equal(t, leases[idx10].Address, net.IP{192, 168, 0, 10})
}

func Test_Database_ReservedAddresses(t *testing.T) {
	db := getTestDatabase(t)
	addReservedIP(db, "192.168.0.10", defaultClientMAC)
	addReservedIP(db, "192.168.0.11", defaultClientMAC)

	assert.Len(t, db.reservedAddresses, 2)

	leases, err := db.ReservedAddresses(ctx)
	assert.Nil(t, err)
	assert.Len(t, leases, 2)

	has10 := false
	has11 := false

	for _, l := range leases {
		switch l.IP.String() {
		case "192.168.0.10":
			has10 = true
		case "192.168.0.11":
			has11 = true
		default:
			t.Fail()
		}

		assert.Equal(t, l.Client, *defaultClient)
	}

	assert.True(t, has10)
	assert.True(t, has11)

}

func Test_Database_Reserve(t *testing.T) {

	// IP address not leasable
	db := getTestDatabase(t)
	assert.Error(t, db.Reserve(ctx, net.IP{10, 9, 0, 100}, *defaultClient))
	assert.Error(t, db.Reserve(ctx, net.IP{192, 168, 0, 255}, *defaultClient))

	// invalid IP
	db = getTestDatabase(t)
	assert.Error(t, db.Reserve(ctx, net.IP{100}, *defaultClient))

	// address already leased to a different client
	db = getTestDatabase(t)
	addLeasedIP(db, "192.168.0.10", "aa:bb:cc:dd:ee:ff")
	assert.Error(t, db.Reserve(ctx, net.IP{192, 168, 0, 10}, *defaultClient))

	// address already leased to us
	db = getTestDatabase(t)
	addLeasedIP(db, "192.168.0.11", defaultClientMAC)
	assert.NoError(t, db.Reserve(ctx, net.IP{192, 168, 0, 11}, *defaultClient))

	// address already reserved for another client
	db = getTestDatabase(t)
	addReservedIP(db, "192.168.0.12", "aa:bb:cc:dd:ee:ff")
	assert.Error(t, db.Reserve(ctx, net.IP{192, 168, 0, 12}, *defaultClient))

	// address already reserved for us
	db = getTestDatabase(t)
	addLeasedIP(db, "192.168.0.13", defaultClientMAC)
	assert.NoError(t, db.Reserve(ctx, net.IP{192, 168, 0, 13}, *defaultClient))
	// TODO(ppacher): test expiration time handling

	db = getTestDatabase(t)
	assert.NoError(t, db.Reserve(ctx, net.IP{192, 168, 0, 14}, *defaultClient))
	key, _ := iprange.IP2Int(net.IP{192, 168, 0, 14})
	assert.Len(t, db.reservedAddresses, 1)
	assert.Equal(t, net.IP{192, 168, 0, 14}, db.reservedAddresses[key].IP)
	assert.Equal(t, defaultClient.HwAddr, db.reservedAddresses[key].Client.HwAddr)
}

func Test_Database_DeleteReservation(t *testing.T) {
	db := getTestDatabase(t)
	addReservedIP(db, "10.1.1.1", defaultClientMAC)
	assert.NoError(t, db.DeleteReservation(ctx, net.IP{10, 1, 1, 1}, defaultClient))

	// no such ip
	db = getTestDatabase(t)
	assert.Error(t, db.DeleteReservation(ctx, net.IP{10, 1, 1, 1}, defaultClient))

	// invalid ip
	db = getTestDatabase(t)
	assert.Error(t, db.DeleteReservation(ctx, net.IP{100, 1, 1, 1, 200}, defaultClient))
	assert.Error(t, db.DeleteReservation(ctx, net.IP{}, defaultClient))

	// wrong mac
	db = getTestDatabase(t)
	addReservedIP(db, "10.1.1.1", "aa:bb:00:cc:11:dd")
	assert.Error(t, db.DeleteReservation(ctx, net.IP{10, 1, 1, 1}, defaultClient))
}
