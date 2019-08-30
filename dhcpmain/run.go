package dhcpmain

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/caddyserver/caddy"
)

var (
	conf       string
	serverType = "dhcpv4"
)

func init() {
	caddy.DefaultConfigFile = "Dhcpfile"
	caddy.Quiet = false

	flag.StringVar(&conf, "conf", "", "Dhcpfile to load (default \""+caddy.DefaultConfigFile+"\")")

	caddy.RegisterCaddyfileLoader("flag", caddy.LoaderFunc(configLoader))
	caddy.SetDefaultCaddyfileLoader("default", caddy.LoaderFunc(defaultLoader))

	caddy.AppName = "dhcp-ng"
	caddy.AppVersion = "v0.1.0"
}

func Run() {
	caddy.TrapSignals()

	fmt.Println(caddy.DescribePlugins())

	dhcpfile, err := caddy.LoadCaddyfile(serverType)
	if err != nil {
		log.Fatal(err)
	}

	instance, err := caddy.Start(dhcpfile)
	if err != nil {
		log.Fatal(err)
	}

	instance.Wait()
}

func configLoader(serverType string) (caddy.Input, error) {
	if conf == "" {
		return nil, nil
	}

	if conf == "stdin" || conf == "-" {
		return caddy.CaddyfileFromPipe(os.Stdin, serverType)
	}

	file, err := ioutil.ReadFile(conf)
	if err != nil {
		return nil, err
	}

	return caddy.CaddyfileInput{
		Contents:       file,
		Filepath:       conf,
		ServerTypeName: serverType,
	}, nil
}

func defaultLoader(serverType string) (caddy.Input, error) {
	conf = caddy.DefaultConfigFile
	return configLoader(serverType)
}