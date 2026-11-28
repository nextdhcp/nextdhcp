// Package caddylog makes zerolog the logger of Caddy. Caddy and its modules
// log with zap: the zerolog log core passes what they log on to zerolog's
// global logger, and Logger returns the zerolog logger that a module logs
// with instead of the zap logger Caddy has for it.
package caddylog

import (
	"fmt"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	caddy.RegisterModule(Core{})
}

// loggerField is the name of the field that holds the name of a logger, which
// is the ID of the Caddy module that logs with it.
const loggerField = "logger"

// Core is the "zerolog" log core: a zapcore.Core that logs with zerolog's
// global logger. Caddy passes what is logged to a log on to the core of the
// log, and still writes it itself, unless the log has a writer that discards
// it, see Configure.
type Core struct {
	// Level is the lowest level to log: trace, debug, info, warn, error,
	// fatal, panic or disabled. It defaults to info.
	Level string `json:"level,omitempty"`

	logger zerolog.Logger
}

// CaddyModule returns the Caddy module information.
func (Core) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.logging.cores.zerolog",
		New: func() caddy.Module { return new(Core) },
	}
}

// Provision sets up the logger of the core.
func (c *Core) Provision(caddy.Context) error {
	level := zerolog.InfoLevel
	if c.Level != "" {
		var err error
		if level, err = zerolog.ParseLevel(c.Level); err != nil {
			return fmt.Errorf("invalid log level %q", c.Level)
		}
	}

	c.logger = log.Logger.Level(level)

	return nil
}

// Enabled implements zapcore.Core.
func (c *Core) Enabled(level zapcore.Level) bool {
	l := zerologLevel(level)

	return l >= c.logger.GetLevel() && l >= zerolog.GlobalLevel()
}

// With implements zapcore.Core.
func (c *Core) With(fields []zapcore.Field) zapcore.Core {
	return &Core{
		Level:  c.Level,
		logger: c.logger.With().Fields(fieldMap(fields)).Logger(),
	}
}

// Check implements zapcore.Core.
func (c *Core) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}

	return checked
}

// Write implements zapcore.Core. It doesn't exit or panic after it logged a
// message of the fatal or panic level: the zap logger of the message does.
func (c *Core) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	e := c.logger.WithLevel(zerologLevel(entry.Level))

	if entry.LoggerName != "" {
		e.Str(loggerField, entry.LoggerName)
	}

	if entry.Caller.Defined {
		e.Str(zerolog.CallerFieldName, entry.Caller.String())
	}

	if len(fields) > 0 {
		e.Fields(fieldMap(fields))
	}

	if entry.Stack != "" {
		e.Str(zerolog.ErrorStackFieldName, entry.Stack)
	}

	e.Msg(entry.Message)

	return nil
}

// Sync implements zapcore.Core. zerolog doesn't buffer what it logs.
func (c *Core) Sync() error {
	return nil
}

// fieldMap returns the fields of a zap logger or message the way zerolog
// takes them.
func fieldMap(fields []zapcore.Field) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}

	return enc.Fields
}

// zerologLevel returns the zerolog level of a zap level.
func zerologLevel(level zapcore.Level) zerolog.Level {
	switch level {
	case zapcore.DebugLevel:
		return zerolog.DebugLevel
	case zapcore.InfoLevel:
		return zerolog.InfoLevel
	case zapcore.WarnLevel:
		return zerolog.WarnLevel
	case zapcore.ErrorLevel, zapcore.DPanicLevel:
		return zerolog.ErrorLevel
	case zapcore.PanicLevel:
		return zerolog.PanicLevel
	case zapcore.FatalLevel:
		return zerolog.FatalLevel
	}

	if level < zapcore.DebugLevel {
		return zerolog.TraceLevel
	}

	return zerolog.NoLevel
}

// Configure makes zerolog log what Caddy and its modules log with zap while
// the configuration cfg runs, from the given level on: the default log of
// cfg, which they all log to, gets the zerolog core, and a writer that
// discards what Caddy writes itself.
func Configure(cfg *caddy.Config, level zerolog.Level) {
	if cfg.Logging == nil {
		cfg.Logging = new(caddy.Logging)
	}

	if cfg.Logging.Logs == nil {
		cfg.Logging.Logs = make(map[string]*caddy.CustomLog)
	}

	l, ok := cfg.Logging.Logs[caddy.DefaultLoggerName]
	if !ok {
		l = new(caddy.CustomLog)
		cfg.Logging.Logs[caddy.DefaultLoggerName] = l
	}

	l.WriterRaw = caddyconfig.JSONModuleObject(caddy.DiscardWriter{}, "output", "discard", nil)
	l.CoreRaw = caddyconfig.JSONModuleObject(Core{Level: level.String()}, "module", "zerolog", nil)
}

// RedirectDefault makes Caddy's default logger, which Caddy logs with while
// no configuration is loaded, log with zerolog's global logger, from the
// given level on. Loading a configuration replaces the default logger with
// the one it configures, see Configure.
func RedirectDefault(level zerolog.Level) {
	logger := zap.New(&Core{logger: log.Logger.Level(level)})

	// Caddy only replaces its default logger with a given one to stop
	// buffering what is logged with it
	caddy.BufferedLog()
	caddy.UnbufferDefaultLogger(logger)
}

// Logger returns the zerolog logger for the module that is provisioned with
// ctx, to log with instead of the zap logger that ctx has for the module. It
// logs with zerolog's global logger, from the level on that Caddy's logging
// configuration lets the module log at, and adds the ID of the module to
// what it logs.
func Logger(ctx caddy.Context) zerolog.Logger {
	return fromZap(ctx.Logger())
}

// Default returns the zerolog logger for what isn't logged by a module, to
// log with instead of Caddy's default logger. It logs with zerolog's global
// logger, from the level on that Caddy's default logger logs at.
func Default() zerolog.Logger {
	return fromZap(caddy.Log())
}

// fromZap returns a logger that logs with zerolog's global logger what l
// would log, by its level, and adds the name of l, if it has one.
func fromZap(l *zap.Logger) zerolog.Logger {
	logger := log.Logger.Level(lowestLevel(l.Core()))

	if name := l.Name(); name != "" {
		logger = logger.With().Str(loggerField, name).Logger()
	}

	return logger
}

// lowestLevel returns the lowest zerolog level to log at for what a zap core
// logs, which is nothing if the core logs at none of zap's levels.
func lowestLevel(core zapcore.LevelEnabler) zerolog.Level {
	for level := zapcore.DebugLevel; level <= zapcore.FatalLevel; level++ {
		if core.Enabled(level) {
			// zerolog's fatal level is below its panic level, unlike zap's
			return min(zerologLevel(level), zerolog.FatalLevel)
		}
	}

	return zerolog.Disabled
}

// Interface guards
var (
	_ caddy.Provisioner = (*Core)(nil)
	_ zapcore.Core      = (*Core)(nil)
)
