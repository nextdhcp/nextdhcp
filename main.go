package main

import (
	"log"

	"github.com/ppacher/dhcp-ng/internal/cmd"
)

func main() {
	if err := cmd.DHCPv4Server.Execute(); err != nil {
		log.Fatal(err)
	}
}
