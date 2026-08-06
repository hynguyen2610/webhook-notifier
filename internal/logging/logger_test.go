package logging

import (
	"log/slog"
	"testing"
)

func TestNewLoggerReturnsLoggerInstance(t *testing.T) {
	// Input: a supported log level string.
	// Outcome: the logger factory returns a usable logger instance.
	logger := NewLogger("debug")
	if logger == nil {
		t.Fatal("expected logger instance")
	}
}

func TestParseLevelRecognizesSupportedAndDefaultLevels(t *testing.T) {
	testCases := []struct {
		name          string
		logLevel      string
		expectedLevel slog.Level
	}{
		{
			name:          "input debug expects debug level",
			logLevel:      "DEBUG",
			expectedLevel: slog.LevelDebug,
		},
		{
			name:          "input warn expects warn level",
			logLevel:      "warn",
			expectedLevel: slog.LevelWarn,
		},
		{
			name:          "input error expects error level",
			logLevel:      " ERROR ",
			expectedLevel: slog.LevelError,
		},
		{
			name:          "input unknown expects info level",
			logLevel:      "trace",
			expectedLevel: slog.LevelInfo,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			level := parseLevel(testCase.logLevel)
			if level != testCase.expectedLevel {
				t.Fatalf("expected level %v, got %v", testCase.expectedLevel, level)
			}
		})
	}
}

