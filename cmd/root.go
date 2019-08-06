package cmd

import (
	"context"
	"log"

	"github.com/ppacher/dhcp-ng/pkg/server"
	"github.com/spf13/cobra"
)

var (
	flagListenAddresses []string
)

// DHCPv4Server is the root cobra command for dhcp-ng
var DHCPv4Server = &cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		s := server.New(server.WithListen(flagListenAddresses...))

		log.Printf("Listening on %s", flagListenAddresses)
		if err := s.Start(context.Background()); err != nil {
			log.Fatal(err)
		}

		log.Printf("Waiting for server to finish")
		if err := s.Wait(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Good bye")
	},
}

func init() {
	flags := DHCPv4Server.Flags()

	flags.StringSliceVarP(&flagListenAddresses, "listen", "l", []string{"127.0.0.1:67"}, "Addresses to listen on")
}
