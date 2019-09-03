package main

//"github.com/nextdhcp/nextdhcp/internal/cmd"
import (
	"github.com/nextdhcp/nextdhcp/dhcpmain"

	// plugin dhcpserver
	_ "github.com/nextdhcp/nextdhcp/core"
)

func main() {
	dhcpmain.Run()
}
