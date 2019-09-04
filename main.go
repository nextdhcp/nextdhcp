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
