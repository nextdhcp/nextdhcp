package dhcpmain

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/core/log/caddylog"
)

var conf string

func init() {
	flag.StringVar(&conf, "conf", "", "Dhcpfile to load (default \""+dhcpserver.DefaultConfigFile+"\")")
}

// Run start NextDHCP and blocks until the server stopped
func Run() {
	flag.Parse()
	setupLogging(os.Stderr)
	caddy.TrapSignals()

	// zerolog also logs what the standard library's logger does, but at the
	// info level, so log errors with it directly
	logger := caddylog.Default()

	filename, body, err := readDhcpfile()
	if err != nil {
		logger.Fatal().Msg(err.Error())
	}

	cfgJSON, err := adapt(filename, body)
	if err != nil {
		logger.Fatal().Msg(err.Error())
	}

	if err := caddy.Load(cfgJSON, true); err != nil {
		logger.Fatal().Msg(err.Error())
	}

	r := newReloader(cfgJSON)

	// SIGUSR1 reloads the Dhcpfile (see caddy.TrapSignals), unless it was
	// read from standard input, which can't be read again
	if filename != os.Stdin.Name() {
		caddy.SetLastConfig(filename, "dhcpfile", r.reload)
	}

	// The process exits once a signal stops it (see caddy.TrapSignals), or
	// with an error once a server stops serving, so that a service manager
	// can restart it
	err = r.wait()

	// Caddy exits with status 0 on SIGINT and SIGTERM, so keep a signal from
	// starting that exit while this one stops NextDHCP. A signal that arrived
	// just before marks Caddy as exiting right away: let that exit finish, as
	// it was asked for.
	signal.Ignore(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	time.Sleep(100 * time.Millisecond)
	if caddy.Exiting() {
		select {}
	}

	// log at the level of the configuration that was loaded
	logger = caddylog.Default()
	logger.Error().Err(err).Msg("exiting because NextDHCP stopped serving")
	if err := caddy.Stop(); err != nil {
		logger.Error().Err(err).Msg("failed to stop")
	}

	os.Exit(1)
}

// adapt adapts the Dhcpfile body, read from filename, to a Caddy JSON
// config.
func adapt(filename string, body []byte) ([]byte, error) {
	cfgJSON, warnings, err := dhcpserver.Adapter{}.Adapt(body, map[string]any{
		"filename": filename,
	})
	logger := caddylog.Default()
	for _, w := range warnings {
		logger.Warn().Msg(w.String())
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

// failer is the part of the dhcp app that Run watches.
type failer interface {
	Failed() <-chan error
}

// reloader replaces the running configuration with the one of the Dhcpfile
// when NextDHCP gets SIGUSR1, and tells Run which dhcp app to watch.
type reloader struct {
	mu      sync.Mutex // held while the running configuration is replaced
	running []byte     // the JSON config that runs

	reloaded chan struct{} // gets a value once a reload loaded a configuration
	failed   chan error    // gets the error of a reload that left none running

	// stop stops the running configuration, load loads one and app returns
	// the dhcp app of the running one. Tests replace them.
	stop func() error
	load func(cfgJSON []byte, forceReload bool) error
	app  func() (failer, error)
}

// newReloader returns a reloader of the running JSON config.
func newReloader(running []byte) *reloader {
	return &reloader{
		running:  running,
		reloaded: make(chan struct{}, 1),
		failed:   make(chan error, 1),
		stop:     caddy.Stop,
		load:     caddy.Load,
		app:      runningApp,
	}
}

// runningApp returns the dhcp app of the running configuration.
func runningApp() (failer, error) {
	app, err := caddy.ActiveContext().AppIfConfigured("dhcp")
	if err != nil {
		return nil, err
	}

	return app.(*dhcpserver.App), nil
}

// reload replaces the running configuration with the one of the Dhcpfile
// file, for SIGUSR1. Caddy starts a new configuration before it stops the
// running one, which fails here, as the servers of the running configuration
// hold the sockets and lease database files that the new ones open. So the
// running configuration is stopped first, once the Dhcpfile was adapted, and
// it is loaded again if the new one fails to load. If that fails too, no
// configuration runs, and Run exits.
func (r *reloader) reload(file, _ string) error {
	body, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	cfgJSON, err := adapt(file, body)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.stop(); err != nil {
		return err
	}

	if err := r.load(cfgJSON, true); err != nil {
		if rollbackErr := r.load(r.running, true); rollbackErr != nil {
			err = fmt.Errorf("failed to load %s, and to load the previous configuration again: %w", file, errors.Join(err, rollbackErr))
			notify(r.failed, err)

			return err
		}

		notify(r.reloaded, struct{}{})

		return fmt.Errorf("failed to load %s, so the previous configuration runs again: %w", file, err)
	}

	r.running = cfgJSON
	notify(r.reloaded, struct{}{})

	return nil
}

// notify sends v to ch unless ch is full: reload runs on the goroutine that
// handles signals, which it must not block, for example for SIGTERM.
func notify[T any](ch chan<- T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// wait returns the error that stops NextDHCP: the error of a server that
// stopped serving, or of a reload that left no configuration running. After
// a reload, it watches the dhcp app of the configuration that runs then.
func (r *reloader) wait() error {
	for {
		// a reload replaces the running app while it holds mu
		r.mu.Lock()
		app, err := r.app()
		r.mu.Unlock()

		if err != nil {
			// a reload that failed may have left no app running
			select {
			case reloadErr := <-r.failed:
				return reloadErr
			default:
				return err
			}
		}

		select {
		case err := <-app.Failed():
			return err
		case err := <-r.failed:
			return err
		case <-r.reloaded:
		}
	}
}
