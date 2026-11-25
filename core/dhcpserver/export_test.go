package dhcpserver

import (
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
