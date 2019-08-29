package cmd

import (

	// import built-in lease database
	"context"
	"log"
	"net"

	"github.com/ppacher/dhcp-ng/internal/utils"
	"github.com/ppacher/dhcp-ng/pkg/handler"
	"github.com/ppacher/dhcp-ng/pkg/lua"
	"github.com/ppacher/dhcp-ng/pkg/server"

	"github.com/spf13/cobra"

	// load the builtin database
	_ "github.com/ppacher/dhcp-ng/pkg/lease/builtin"
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

		var subnets []handler.SubnetConfig

		for _, def := range runner.Subnets() {
			s, err := utils.SubnetConfigFromLua(runner, def)
			if err != nil {
				log.Fatal(err)
			}

			subnets = append(subnets, *s)
		}

		serve := handler.NewV4(handler.Option{
			Subnets: subnets,
		})

		listenIPs := make([]net.IP, 0, len(subnets))

		for _, s := range subnets {
			listenIPs = append(listenIPs, s.IP)
		}

		srv := server.New(serve.Serve, listenIPs)
		if err := srv.Start(context.Background()); err != nil {
			log.Fatal(err)
		}

		if err := srv.Wait(); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	flags := DHCPv4Server.Flags()

	flags.StringVarP(&configFile, "config", "c", "rc.lua", "Path to configuration file")
}
