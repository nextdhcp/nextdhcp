//go:build linux

// NextDHCP only builds for Linux: it talks to DHCP clients through AF_PACKET
// sockets (github.com/mdlayher/packet), which other systems don't have. The
// other packages still build anywhere, so their code and tests compile there.

package main

//"github.com/nextdhcp/nextdhcp/internal/cmd"
import (
	"github.com/nextdhcp/nextdhcp/dhcpmain"

	// plugin dhcpserver as well all all supported plugins
	// and the default database
	_ "github.com/nextdhcp/nextdhcp/core"
)

func main() {
	dhcpmain.Run()
}
