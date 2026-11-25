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
		"debug":   "DEBUG",
		"info":    "INFO",
		"warn":    "WARN",
		"warning": "WARN",
		"error":   "ERROR",
		"fatal":   "FATAL",
		"DEBUG":   "DEBUG",
	}

	for level, expected := range levels {
		c := test.CreateTestBed(t, "log "+level)
		require.NoError(t, setupLogging(c), level)

		require.NotNil(t, c.Config.Logging, level)
		require.Contains(t, c.Config.Logging.Logs, caddy.DefaultLoggerName, level)
		assert.Equal(t, expected, c.Config.Logging.Logs[caddy.DefaultLoggerName].Level, level)
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
