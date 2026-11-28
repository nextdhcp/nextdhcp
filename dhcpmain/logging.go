package dhcpmain

import (
	"io"
	stdlog "log"
	"os"
	"strings"

	"github.com/nextdhcp/nextdhcp/core/log/caddylog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
)

const (
	// timeFormat is the format of the time of a log message: RFC 3339 with
	// milliseconds.
	timeFormat = "2006-01-02T15:04:05.000Z07:00"

	// consoleTimeFormat is the format of the time of a log message that is
	// logged to a terminal.
	consoleTimeFormat = "2006-01-02 15:04:05.000"
)

// setupLogging makes zerolog log everything that NextDHCP logs to out, as
// JSON, or human-readable if out is a terminal. That's what NextDHCP logs
// itself, with zerolog, what Caddy logs, with zap, and what dependencies log
// with the standard library's logger.
func setupLogging(out *os.File) {
	zerolog.TimeFieldFormat = timeFormat
	log.Logger = newLogger(out, term.IsTerminal(int(out.Fd())))

	stdlog.SetFlags(0)
	stdlog.SetOutput(stdWriter{})

	// Until the Dhcpfile is loaded, which configures the log level, Caddy
	// only logs warnings and errors. Otherwise it would log, when it loads
	// the Dhcpfile, that it redirects its default logger to a writer that
	// discards, which is how the configuration of the Dhcpfile makes zerolog
	// log instead of Caddy.
	caddylog.RedirectDefault(zerolog.WarnLevel)
}

// newLogger returns a logger that logs to out, with the time: as JSON, or
// human-readable for a console.
func newLogger(out io.Writer, console bool) zerolog.Logger {
	if console {
		out = zerolog.ConsoleWriter{Out: out, TimeFormat: consoleTimeFormat}
	}

	return zerolog.New(out).With().Timestamp().Logger()
}

// stdWriter logs what the standard library's logger writes with zerolog's
// global logger. It logs at the info level, as the standard library's logger
// doesn't have levels.
type stdWriter struct{}

// Write implements io.Writer. The standard library's logger writes each
// message with a single call.
func (stdWriter) Write(p []byte) (int, error) {
	log.Info().Msg(strings.TrimSuffix(string(p), "\n"))

	return len(p), nil
}
