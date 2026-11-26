package dhcpserver

import (
	"context"
	"net"

	"github.com/nextdhcp/nextdhcp/core/log"
)

// SetListenDHCP replaces the function servers open their connection with,
// and returns a function that restores it.
func SetListenDHCP(f func(l log.Logger, ip net.IP, ifc *net.Interface) (net.PacketConn, error)) (restore func()) {
	orig := listenDHCP
	listenDHCP = f

	return func() { listenDHCP = orig }
}

// SetProbeAddress replaces the function that servers ask whether a device
// uses an address with, and returns a function that restores it.
func SetProbeAddress(f func(ctx context.Context, ifc *net.Interface, ip, addr net.IP) (bool, error)) (restore func()) {
	orig := probeAddress
	probeAddress = f

	return func() { probeAddress = orig }
}
