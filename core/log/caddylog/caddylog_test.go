package caddylog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	caddy.RegisterModule(testApp{})
}

// testApp is a Caddy app that calls provisioned when it is provisioned.
type testApp struct{}

// provisioned is called with the context a testApp is provisioned with.
var provisioned func(ctx caddy.Context)

func (testApp) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddylog_test",
		New: func() caddy.Module { return new(testApp) },
	}
}

func (*testApp) Provision(ctx caddy.Context) error {
	if provisioned != nil {
		provisioned(ctx)
	}

	return nil
}

func (*testApp) Start() error { return nil }
func (*testApp) Stop() error  { return nil }

// capture makes zerolog's global logger log to the returned buffer, without
// the time, until the test ends.
func capture(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := new(bytes.Buffer)

	orig := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = orig })

	return buf
}

// logged returns the messages that were logged to buf, and empties buf.
func logged(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var messages []map[string]any

	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &m), scanner.Text())
		messages = append(messages, m)
	}
	require.NoError(t, scanner.Err())

	buf.Reset()

	return messages
}

// newCore returns a provisioned core that logs from level on.
func newCore(t *testing.T, level string) *Core {
	t.Helper()

	c := &Core{Level: level}
	require.NoError(t, c.Provision(caddy.Context{}))

	return c
}

func TestCoreLogsWithZerolog(t *testing.T) {
	buf := capture(t)
	logger := zap.New(newCore(t, "debug"))

	logger.Debug("a debug message")
	logger.Info("an info message")
	logger.Warn("a warning")
	logger.Error("an error")
	logger.DPanic("a critical error")
	assert.PanicsWithValue(t, "a panic", func() { logger.Panic("a panic") })

	assert.Equal(t, []map[string]any{
		{"level": "debug", "message": "a debug message"},
		{"level": "info", "message": "an info message"},
		{"level": "warn", "message": "a warning"},
		{"level": "error", "message": "an error"},
		{"level": "error", "message": "a critical error"},
		{"level": "panic", "message": "a panic"},
	}, logged(t, buf))
}

func TestCoreLogsFields(t *testing.T) {
	buf := capture(t)
	logger := zap.New(newCore(t, "debug"))

	logger.Named("dhcp").Named("handlers").Info("a message",
		zap.String("string", "value"),
		zap.Int("int", 42),
		zap.Bool("bool", true),
		zap.Duration("duration", 1500*time.Millisecond),
		zap.Error(errors.New("failed")),
		zap.Strings("strings", []string{"a", "b"}),
		zap.Namespace("namespace"),
		zap.String("nested", "value"),
	)

	assert.Equal(t, []map[string]any{{
		"level":     "info",
		"logger":    "dhcp.handlers",
		"message":   "a message",
		"string":    "value",
		"int":       float64(42),
		"bool":      true,
		"duration":  float64(1500),
		"error":     "failed",
		"strings":   []any{"a", "b"},
		"namespace": map[string]any{"nested": "value"},
	}}, logged(t, buf))

	// the fields of a logger are logged with each of its messages, and not
	// with those of the logger it was made from
	child := logger.With(zap.String("signal", "SIGUSR1"))
	child.Info("with a field", zap.String("file", "Dhcpfile"))
	child.With(zap.String("adapter", "dhcpfile")).Warn("with two fields")
	logger.Info("without fields")

	assert.Equal(t, []map[string]any{
		{"level": "info", "message": "with a field", "signal": "SIGUSR1", "file": "Dhcpfile"},
		{"level": "warn", "message": "with two fields", "signal": "SIGUSR1", "adapter": "dhcpfile"},
		{"level": "info", "message": "without fields"},
	}, logged(t, buf))
}

func TestCoreLogsCallerAndStack(t *testing.T) {
	buf := capture(t)
	logger := zap.New(newCore(t, "debug"), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	logger.Info("with caller")
	logger.Error("with caller and stack")

	messages := logged(t, buf)
	require.Len(t, messages, 2)

	assert.Contains(t, messages[0]["caller"], "caddylog_test.go:")
	assert.NotContains(t, messages[0], "stack")

	assert.Contains(t, messages[1]["caller"], "caddylog_test.go:")
	assert.Contains(t, messages[1]["stack"], "TestCoreLogsCallerAndStack")
}

func TestCoreLevel(t *testing.T) {
	// the messages that are logged from each level on, by a logger that logs
	// one message at each level
	levels := map[string][]any{
		"trace":    {"debug", "info", "warn", "error", "fatal"},
		"debug":    {"debug", "info", "warn", "error", "fatal"},
		"DEBUG":    {"debug", "info", "warn", "error", "fatal"},
		"default":  {"info", "warn", "error", "fatal"},
		"info":     {"info", "warn", "error", "fatal"},
		"warn":     {"warn", "error", "fatal"},
		"error":    {"error", "fatal"},
		"fatal":    {"fatal"},
		"disabled": nil,
	}

	for level, expected := range levels {
		t.Run(level, func(t *testing.T) {
			buf := capture(t)

			if level == "default" {
				level = ""
			}

			// zap exits after it logged a message of the fatal level, unless
			// it is told to do something else
			logger := zap.New(newCore(t, level), zap.WithFatalHook(zapcore.WriteThenGoexit))

			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")

			done := make(chan struct{})
			go func() {
				defer close(done)
				logger.Fatal("fatal")
			}()
			<-done

			var messages []any
			for _, m := range logged(t, buf) {
				assert.Equal(t, m["level"], m["message"])
				messages = append(messages, m["message"])
			}

			assert.Equal(t, expected, messages)
		})
	}

	t.Run("invalid", func(t *testing.T) {
		c := &Core{Level: "verbose"}
		assert.ErrorContains(t, c.Provision(caddy.Context{}), `invalid log level "verbose"`)
	})
}

// TestCoreWritesFatalWithoutExiting checks that the core leaves exiting
// after a message of the fatal level to the zap logger of the message, which
// first lets all its cores log the message.
func TestCoreWritesFatalWithoutExiting(t *testing.T) {
	buf := capture(t)
	c := newCore(t, "info")

	require.NoError(t, c.Write(zapcore.Entry{Level: zapcore.FatalLevel, Message: "a fatal error"}, nil))

	assert.Equal(t, []map[string]any{{"level": "fatal", "message": "a fatal error"}}, logged(t, buf))
}

// coreConfig is the configuration of a log core.
type coreConfig struct {
	Module string `json:"module"`
	Level  string `json:"level"`
}

func TestConfigure(t *testing.T) {
	cfg := new(caddy.Config)

	Configure(cfg, zerolog.InfoLevel)

	require.NotNil(t, cfg.Logging)
	require.Contains(t, cfg.Logging.Logs, caddy.DefaultLoggerName)
	l := cfg.Logging.Logs[caddy.DefaultLoggerName]

	assert.JSONEq(t, `{"output": "discard"}`, string(l.WriterRaw))
	assert.JSONEq(t, `{"module": "zerolog", "level": "info"}`, string(l.CoreRaw))

	// only the level changes in a configuration that was configured before
	l.Exclude = []string{"admin"}

	Configure(cfg, zerolog.DebugLevel)

	require.Same(t, l, cfg.Logging.Logs[caddy.DefaultLoggerName])
	assert.JSONEq(t, `{"output": "discard"}`, string(l.WriterRaw))
	assert.Equal(t, []string{"admin"}, l.Exclude)

	var core coreConfig
	require.NoError(t, json.Unmarshal(l.CoreRaw, &core))
	assert.Equal(t, coreConfig{Module: "zerolog", Level: "debug"}, core)
}

// testConfig returns a configuration with a testApp, whose logging is
// configured to log from level on.
func testConfig(level zerolog.Level) *caddy.Config {
	cfg := &caddy.Config{
		AppsRaw: caddy.ModuleMap{"caddylog_test": json.RawMessage(`{}`)},
	}
	Configure(cfg, level)

	return cfg
}

// TestConfiguredLogging checks that, with a configuration that Configure
// configured, the zerolog logger of a module, the zap logger of the module
// and Caddy's default logger all log with zerolog, from the configured level
// on.
func TestConfiguredLogging(t *testing.T) {
	// the levels of the messages that are logged from each level on
	levels := map[zerolog.Level][]string{
		zerolog.DebugLevel: {"debug", "info", "warn", "error"},
		zerolog.InfoLevel:  {"info", "warn", "error"},
		zerolog.WarnLevel:  {"warn", "error"},
		zerolog.ErrorLevel: {"error"},
		zerolog.FatalLevel: nil,
		zerolog.Disabled:   nil,
	}

	for level, expected := range levels {
		t.Run(level.String(), func(t *testing.T) {
			buf := capture(t)

			var moduleLevel zerolog.Level
			provisioned = func(ctx caddy.Context) {
				l := Logger(ctx)
				moduleLevel = l.GetLevel()

				l.Debug().Msg("zerolog")
				l.Info().Msg("zerolog")
				l.Warn().Msg("zerolog")
				l.Error().Msg("zerolog")

				ctx.Logger().Debug("zap")
				ctx.Logger().Info("zap")
				ctx.Logger().Warn("zap")
				ctx.Logger().Error("zap")
			}
			t.Cleanup(func() { provisioned = nil })

			require.NoError(t, caddy.Validate(testConfig(level)))

			assert.Equal(t, level, moduleLevel)

			var messages []map[string]any
			for _, logger := range []string{"zerolog", "zap"} {
				for _, l := range expected {
					messages = append(messages, map[string]any{
						"level":   l,
						"logger":  "caddylog_test",
						"message": logger,
					})
				}
			}
			assert.Equal(t, messages, logged(t, buf))

			// The default logger is the one of the configuration that was
			// loaded last, also once it doesn't run anymore
			def := Default()
			assert.Equal(t, level, def.GetLevel())

			def.Debug().Msg("zerolog")
			def.Info().Msg("zerolog")
			def.Warn().Msg("zerolog")
			def.Error().Msg("zerolog")

			caddy.Log().Debug("zap")
			caddy.Log().Info("zap")
			caddy.Log().Warn("zap")
			caddy.Log().Error("zap")

			messages = nil
			for _, logger := range []string{"zerolog", "zap"} {
				for _, l := range expected {
					messages = append(messages, map[string]any{"level": l, "message": logger})
				}
			}
			assert.Equal(t, messages, logged(t, buf))
		})
	}
}

// TestLoggerOfExcludedModule checks that a module whose logs Caddy's logging
// configuration excludes doesn't log with zerolog either.
func TestLoggerOfExcludedModule(t *testing.T) {
	buf := capture(t)

	cfg := testConfig(zerolog.DebugLevel)
	cfg.Logging.Logs[caddy.DefaultLoggerName].Exclude = []string{"caddylog_test"}

	var moduleLevel zerolog.Level
	provisioned = func(ctx caddy.Context) {
		l := Logger(ctx)
		moduleLevel = l.GetLevel()

		l.Error().Msg("zerolog")
		ctx.Logger().Error("zap")
	}
	t.Cleanup(func() { provisioned = nil })

	require.NoError(t, caddy.Validate(cfg))

	assert.Equal(t, zerolog.Disabled, moduleLevel)
	assert.Empty(t, logged(t, buf))
}

// TestLoggerWithoutConfig checks the logger of a module that is provisioned
// without a configuration, as tests do: Caddy logs everything then.
func TestLoggerWithoutConfig(t *testing.T) {
	buf := capture(t)

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	l := Logger(ctx)
	assert.Equal(t, zerolog.DebugLevel, l.GetLevel())

	l.Debug().Msg("a debug message")

	assert.Equal(t, []map[string]any{{"level": "debug", "message": "a debug message"}}, logged(t, buf))
}

func TestRedirectDefault(t *testing.T) {
	buf := capture(t)

	RedirectDefault(zerolog.WarnLevel)

	caddy.Log().Info("not logged")
	caddy.Log().Named("admin").Warn("a warning", zap.String("address", "localhost:2019"))
	caddy.Log().Error("an error", zap.Error(errors.New("failed")))

	def := Default()
	assert.Equal(t, zerolog.WarnLevel, def.GetLevel())

	def.Info().Msg("not logged either")
	def.Warn().Msg("a warning of NextDHCP")

	assert.Equal(t, []map[string]any{
		{"level": "warn", "logger": "admin", "message": "a warning", "address": "localhost:2019"},
		{"level": "error", "message": "an error", "error": "failed"},
		{"level": "warn", "message": "a warning of NextDHCP"},
	}, logged(t, buf))
}

// TestRedirectedNote checks why NextDHCP redirects Caddy's default logger at
// the warn level before it loads the Dhcpfile: the first configuration that
// Configure configured makes Caddy log, at the info level and with the
// default logger it had before, that it redirects the default logger to a
// writer that discards.
func TestRedirectedNote(t *testing.T) {
	const note = "redirected default logger"

	for level, noted := range map[zerolog.Level]bool{
		zerolog.InfoLevel: true,
		zerolog.WarnLevel: false,
	} {
		t.Run(level.String(), func(t *testing.T) {
			buf := capture(t)

			// like when NextDHCP starts, Caddy's default log writes to
			// standard error
			require.NoError(t, caddy.Validate(new(caddy.Config)))

			RedirectDefault(level)
			require.NoError(t, caddy.Validate(testConfig(zerolog.DebugLevel)))

			assert.Equal(t, noted, strings.Contains(buf.String(), note), buf.String())
		})
	}
}
