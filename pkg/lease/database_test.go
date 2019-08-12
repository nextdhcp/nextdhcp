package lease

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	ctx           = context.Background()
	defaultClient = &Client{
		HwAddr:   net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		Hostname: "test-client",
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
	key, _ := IPToInt(i.To4())
	db.reservedAddresses[key] = ReservedAddress{
		Client: Client{HwAddr: m},
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
	key, _ := IPToInt(i.To4())
	db.leasedAddresses[key] = &Lease{
		Client:  Client{HwAddr: m},
		Address: i.To4(),
		Expires: time.Now().Add(time.Minute),
	}
	db.leasedAddressesByClient[m.String()] = key

	return key
}

func getTestDatabase(t *testing.T) *database {
	_, ipNet, _ := net.ParseCIDR("192.168.0.1/24")
	ranges := []*IPRange{
		&IPRange{
			Start: net.IP{192, 168, 0, 10},
			End:   net.IP{192, 168, 0, 15},
		},
	}
	db := NewDatabase(ipNet, ranges, WithRange(&IPRange{
		Start: net.IP{192, 168, 0, 12},
		End:   net.IP{192, 168, 0, 20},
	}))

	return db.(*database)
}

func Test_Database_FindAddress_LeaseAvailable(t *testing.T) {
	db := getTestDatabase(t)
	addReservedIP(db, "192.168.0.10", "00:11:22:33:44:55")
	addLeasedIP(db, "192.168.0.11", "00:11:22:33:44:55")

	ip, err := db.FindAddress(ctx, defaultClient)

	assert.Nil(t, err)
	assert.Equal(t, net.IP{192, 168, 0, 12}, ip)
}

func Test_Database_FindAddress_ReservationAvailable(t *testing.T) {
	db := getTestDatabase(t)
	addReservedIP(db, "192.168.0.15", defaultClientMAC)

	ip, err := db.FindAddress(ctx, defaultClient)
	assert.Nil(t, err)
	assert.Equal(t, net.IP{192, 168, 0, 15}, ip)
}

func Test_Database_FindAddress_LeasedAddressAvailable(t *testing.T) {
	db := getTestDatabase(t)
	addLeasedIP(db, "192.168.0.15", defaultClientMAC)

	ip, err := db.FindAddress(ctx, defaultClient)
	assert.Nil(t, err)
	assert.Equal(t, net.IP{192, 168, 0, 15}, ip)
}

func Test_Database_NoLeaseAvailable(t *testing.T) {
	db := getTestDatabase(t)
	for i := 10; i <= 20; i++ {
		if i%2 == 0 {
			addLeasedIP(db, fmt.Sprintf("192.168.0.%d", i), fmt.Sprintf("00:11:22:33:44:%d", i))
		} else {
			addReservedIP(db, fmt.Sprintf("192.168.0.%d", i), fmt.Sprintf("00:11:22:33:44:%d", i))
		}
	}
	ip, err := db.FindAddress(ctx, defaultClient)
	assert.NotNil(t, err)
	assert.Nil(t, ip)
}

func Test_Database_AddRange(t *testing.T) {
	db := getTestDatabase(t)

	r1 := &IPRange{
		Start: net.IP{192, 168, 0, 100},
		End:   net.IP{192, 168, 0, 200},
	}

	r2 := &IPRange{
		Start: net.IP{192, 168, 0, 20},
		End:   net.IP{192, 168, 0, 30},
	}

	db.AddRange(r1, r2)

	assert.Len(t, db.ranges, 2)
	assert.Equal(t, net.IP{192, 168, 0, 10}.String(), db.ranges[0].Start.String())
	assert.Equal(t, net.IP{192, 168, 0, 30}.String(), db.ranges[0].End.String())

	assert.Equal(t, net.IP{192, 168, 0, 100}.String(), db.ranges[1].Start.String())
	assert.Equal(t, net.IP{192, 168, 0, 200}.String(), db.ranges[1].End.String())
}
