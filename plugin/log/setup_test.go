package log

import (
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogSetup(t *testing.T) {
	levels := map[string]string{
		"debug":   "debug",
		"info":    "info",
		"warn":    "warn",
		"warning": "warn",
		"error":   "error",
		"fatal":   "fatal",
		"DEBUG":   "debug",
	}

	for level, expected := range levels {
		c := test.CreateTestBed(t, "log "+level)
		require.NoError(t, setupLogging(c), level)

		require.NotNil(t, c.Config.Logging, level)
		require.Contains(t, c.Config.Logging.Logs, caddy.DefaultLoggerName, level)

		// zerolog logs what is logged to Caddy's default log, from the level
		// on, instead of Caddy
		l := c.Config.Logging.Logs[caddy.DefaultLoggerName]
		assert.JSONEq(t, `{"output": "discard"}`, string(l.WriterRaw), level)
		assert.JSONEq(t, `{"module": "zerolog", "level": "`+expected+`"}`, string(l.CoreRaw), level)
	}

	for _, input := range []string{
		"log",
		"log verbose",
		"log debug\nlog info",
		"log debug info",
	} {
		c := test.CreateTestBed(t, input)
		assert.Error(t, setupLogging(c), input)
	}
}
