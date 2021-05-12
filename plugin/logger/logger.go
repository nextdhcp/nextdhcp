package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

const (
	level       = "level"
	format      = "format"
	output_file = "output-file"

	jsonFormat = "json"
)

type parseLogField func(string)

var (
	// Log Entry application logger
	Log *logrus.Entry

	// Logger application logger
	Logger *logrus.Logger

	logFieldMap = map[string]parseLogField{
		level:       parseLogLevel,
		format:      parseLogFormat,
		output_file: parseOutputFile,
	}
)

func initLog() {
	Logger.SetLevel(logrus.DebugLevel)
	Logger.SetFormatter(&logrus.TextFormatter{})
	Logger.SetOutput(os.Stdout)
}

// AddDefaultFields - use to set permanent fields in logger
func AddDefaultFields(fields map[string]interface{}) {
	Log = Log.WithFields(fields)
}

func parseLog(name string, values []string) {
	if fn, exist := logFieldMap[name]; exist {
		fn(values[0])
	}
}

func parseLogLevel(level string) {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	Logger.SetLevel(logLevel)
}

func parseLogFormat(format string) {
	if format == jsonFormat {
		Logger.SetFormatter(&logrus.JSONFormatter{})
	}
}

func parseOutputFile(outputFile string) {
	file, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		return
	}
	Logger.SetOutput(file)
}
