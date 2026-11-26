package dhcpmain

import (
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"go.uber.org/zap"
)

var conf string

func init() {
	flag.StringVar(&conf, "conf", "", "Dhcpfile to load (default \""+dhcpserver.DefaultConfigFile+"\")")
}

// Run start NextDHCP and blocks until the server stopped
func Run() {
	flag.Parse()
	caddy.TrapSignals()

	// Caddy's logger also logs what the standard library's logger does, but
	// at the info level, so log errors with it directly
	cfgJSON, err := loadConfig()
	if err != nil {
		caddy.Log().Fatal(err.Error())
	}

	if err := caddy.Load(cfgJSON, true); err != nil {
		caddy.Log().Fatal(err.Error())
	}

	app, err := caddy.ActiveContext().AppIfConfigured("dhcp")
	if err != nil {
		caddy.Log().Fatal(err.Error())
	}

	// The process exits once a signal stops it (see caddy.TrapSignals), or
	// with an error once a server stops serving, so that a service manager
	// can restart it
	err = <-app.(*dhcpserver.App).Failed()

	// Caddy exits with status 0 on SIGINT and SIGTERM, so keep a signal from
	// starting that exit while this one stops NextDHCP. A signal that arrived
	// just before marks Caddy as exiting right away: let that exit finish, as
	// it was asked for.
	signal.Ignore(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	time.Sleep(100 * time.Millisecond)
	if caddy.Exiting() {
		select {}
	}

	caddy.Log().Error("exiting because a server stopped serving", zap.Error(err))
	if err := caddy.Stop(); err != nil {
		caddy.Log().Error("failed to stop", zap.Error(err))
	}

	os.Exit(1)
}

// loadConfig reads the Dhcpfile and adapts it to a Caddy JSON config.
func loadConfig() ([]byte, error) {
	filename, body, err := readDhcpfile()
	if err != nil {
		return nil, err
	}

	cfgJSON, warnings, err := dhcpserver.Adapter{}.Adapt(body, map[string]any{
		"filename": filename,
	})
	for _, w := range warnings {
		caddy.Log().Warn(w.String())
	}

	return cfgJSON, err
}

func readDhcpfile() (string, []byte, error) {
	if conf == "stdin" || conf == "-" {
		// Like Caddy v1, read standard input only if it isn't a terminal
		// and use the default Dhcpfile otherwise.
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			body, err := io.ReadAll(os.Stdin)
			return os.Stdin.Name(), body, err
		}

		conf = ""
	}

	if conf == "" {
		conf = dhcpserver.DefaultConfigFile
	}

	body, err := os.ReadFile(conf)

	return conf, body, err
}
