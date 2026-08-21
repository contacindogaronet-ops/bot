package utils

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// InitLogger configures and returns a zerolog.Logger optimized for zero-alloc and clear CLI output.
func InitLogger(levelStr string, pretty bool) zerolog.Logger {
	var lvl zerolog.Level
	switch strings.ToLower(levelStr) {
	case "trace":
		lvl = zerolog.TraceLevel
	case "debug":
		lvl = zerolog.DebugLevel
	case "info":
		lvl = zerolog.InfoLevel
	case "warn":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(lvl)
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var output io.Writer = os.Stdout
	if pretty {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05.000",
			NoColor:    false,
		}
	}

	logger := zerolog.New(output).With().
		Timestamp().
		Str("app", "jargo-userbot").
		Logger()

	return logger
}
