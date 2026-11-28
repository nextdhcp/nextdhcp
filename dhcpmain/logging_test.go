package dhcpmain

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	stdlog "log"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/core/log/caddylog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	// all directives and lease database drivers
	_ "github.com/nextdhcp/nextdhcp/core"
)

// restoreLogging restores the loggers that setupLogging sets up when the
// test ends.
func restoreLogging(t *testing.T) {
	t.Helper()

	logger, format := log.Logger, zerolog.TimeFieldFormat
	flags, writer := stdlog.Flags(), stdlog.Writer()

	t.Cleanup(func() {
		log.Logger, zerolog.TimeFieldFormat = logger, format

		stdlog.SetFlags(flags)
		stdlog.SetOutput(writer)

		// Caddy's default logger can't be restored: let it log with the
		// restored logger of zerolog
		caddylog.RedirectDefault(zerolog.WarnLevel)
	})
}

// logFile returns a file for setupLogging to log to.
func logFile(t *testing.T) *os.File {
	t.Helper()

	out, err := os.Create(filepath.Join(t.TempDir(), "log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = out.Close() })

	return out
}

// logged returns the messages that were logged to out, without their time,
// after checking it.
func logged(t *testing.T, out *os.File) []map[string]any {
	t.Helper()

	content, err := os.ReadFile(out.Name())
	require.NoError(t, err)

	var messages []map[string]any

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &m), scanner.Text())

		// each message has its time, with milliseconds
		require.IsType(t, "", m["time"], scanner.Text())
		at, err := time.Parse(timeFormat, m["time"].(string))
		require.NoError(t, err, scanner.Text())
		assert.WithinDuration(t, time.Now(), at, time.Minute)
		assert.Regexp(t, `\.\d{3}(Z|[+-]\d{2}:\d{2})$`, m["time"])
		delete(m, "time")

		messages = append(messages, m)
	}
	require.NoError(t, scanner.Err())

	return messages
}

func TestSetupLogging(t *testing.T) {
	restoreLogging(t)

	out := logFile(t)
	setupLogging(out)

	// NextDHCP logs with zerolog
	log.Debug().Str("subnet", "10.0.0.1/24").Msg("logged with zerolog")

	// dependencies may log with the standard library's logger
	stdlog.Printf("logged with the standard library's %s", "logger")

	// Caddy logs with zap. It logs warnings and errors until the Dhcpfile is
	// loaded
	caddy.Log().Info("not logged")
	caddy.Log().Named("admin").Warn("logged with zap", zap.Error(errors.New("failed")))

	// so does NextDHCP for what it doesn't log for a server
	logger := caddylog.Default()
	logger.Info().Msg("not logged either")
	logger.Warn().Msg("a warning about the Dhcpfile")

	assert.Equal(t, []map[string]any{
		{"level": "debug", "subnet": "10.0.0.1/24", "message": "logged with zerolog"},
		{"level": "info", "message": "logged with the standard library's logger"},
		{"level": "warn", "logger": "admin", "error": "failed", "message": "logged with zap"},
		{"level": "warn", "message": "a warning about the Dhcpfile"},
	}, logged(t, out))
}

// TestLoggingOfDhcpfile checks what is logged once the configuration of a
// Dhcpfile is loaded: everything, from the level on that the Dhcpfile sets,
// and nothing about how its configuration makes zerolog log.
func TestLoggingOfDhcpfile(t *testing.T) {
	for level, expected := range map[string][]map[string]any{
		"debug": {
			{"level": "debug", "logger": "dhcp", "message": "plugin (0) lease setup"},
			{"level": "debug", "message": "a debug message of Caddy"},
			{"level": "info", "message": "logged with the standard library's logger"},
			{"level": "warn", "message": "a warning of NextDHCP"},
			{"level": "error", "message": "an error of NextDHCP"},
		},
		"error": {
			{"level": "info", "message": "logged with the standard library's logger"},
			{"level": "error", "message": "an error of NextDHCP"},
		},
	} {
		t.Run(level, func(t *testing.T) {
			restoreLogging(t)

			// like when NextDHCP starts, Caddy's default log writes to
			// standard error
			require.NoError(t, caddy.Validate(new(caddy.Config)))

			out := logFile(t)
			setupLogging(out)

			cfgJSON, err := adapt("Dhcpfile", []byte("127.0.0.1/8 {\n\tdatabase memory\n\tlease 1h\n\tlog "+level+"\n}\n"))
			require.NoError(t, err)

			// provisioning the configuration sets up its logging, like loading
			// it, without serving the subnet
			cfg := new(caddy.Config)
			require.NoError(t, json.Unmarshal(cfgJSON, cfg))
			require.NoError(t, caddy.Validate(cfg))

			caddy.Log().Debug("a debug message of Caddy")
			stdlog.Print("logged with the standard library's logger")

			logger := caddylog.Default()
			logger.Warn().Msg("a warning of NextDHCP")
			logger.Error().Msg("an error of NextDHCP")

			assert.Equal(t, expected, logged(t, out))
		})
	}
}

// TestNewLoggerConsole checks what is logged to a terminal, which only shows
// the time of a message the way it should if it can be parsed.
func TestNewLoggerConsole(t *testing.T) {
	restoreLogging(t)
	t.Setenv("NO_COLOR", "1")

	zerolog.TimeFieldFormat = timeFormat

	var buf bytes.Buffer
	logger := newLogger(&buf, true)

	logger.Info().Str("logger", "dhcp.handlers.range").Msg("a message")

	line := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3} INF a message logger=dhcp\.handlers\.range\n$`)
	assert.Regexp(t, line, buf.String())
}
