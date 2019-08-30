package main

//"github.com/ppacher/dhcp-ng/internal/cmd"
import (
	"github.com/ppacher/dhcp-ng/dhcpmain"

	// plugin dhcpserver
	_ "github.com/ppacher/dhcp-ng/core"
)

func main() {
	dhcpmain.Run()
}
