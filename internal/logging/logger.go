package logging

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(logLevel string) *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(parseLevel(logLevel))

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

func parseLevel(logLevel string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(logLevel)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
