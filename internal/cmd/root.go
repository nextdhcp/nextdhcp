package cmd

import (

	// import built-in lease database
	"log"

	_ "github.com/ppacher/dhcp-ng/pkg/lease/builtin"
	"github.com/ppacher/dhcp-ng/pkg/lua"

	"github.com/spf13/cobra"
)

var (
	configFile string
)

// DHCPv4Server is the root cobra command for dhcp-ng
var DHCPv4Server = &cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		runner, err := lua.NewFromFile(configFile)
		if err != nil {
			log.Fatal(err)
		}

		log.Println(runner.Plugins())
		log.Println(runner.Subnets())
	},
}

func init() {
	flags := DHCPv4Server.Flags()

	flags.StringVarP(&configFile, "config", "c", "rc.lua", "Path to configuration file")
}
