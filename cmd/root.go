package cmd

import (
	"context"
	"log"
	"net"

	"github.com/ppacher/dhcp-ng/pkg/server"
	_ "github.com/ppacher/dhcp-ng/pkg/lease/builtin"
	"github.com/spf13/cobra"
)

var (
	flagListenAddresses []string
)

// DHCPv4Server is the root cobra command for dhcp-ng
var DHCPv4Server = &cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		addresses := make([]net.IP, len(flagListenAddresses))

		for i, a := range flagListenAddresses {
			ip := net.ParseIP(a)
			if ip == nil {
				log.Fatalf("Failed to parse IP: %s", a)
			}

			addresses[i] = ip
		}

		s := server.New(server.WithListen(addresses...))

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
