package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadNotifierConfigUsesDefaultsWhenEnvironmentIsUnset(t *testing.T) {
	// Input: only the required PostgreSQL DSN is set.
	// Outcome: notifier config loads with the documented default values.
	t.Setenv("NOTIFIER_POSTGRES_DSN", "postgres://example")

	notifierConfig, loadError := LoadNotifierConfig()
	if loadError != nil {
		t.Fatalf("load notifier config: %v", loadError)
	}

	if notifierConfig.HTTPAddress != ":28080" {
		t.Fatalf("expected default HTTP address, got %s", notifierConfig.HTTPAddress)
	}
	if notifierConfig.WorkerCount != defaultNotifierWorkerCount {
		t.Fatalf("expected default worker count %d, got %d", defaultNotifierWorkerCount, notifierConfig.WorkerCount)
	}
	if notifierConfig.RequestTimeout != defaultNotifierRequestTimeout {
		t.Fatalf("expected default request timeout %s, got %s", defaultNotifierRequestTimeout, notifierConfig.RequestTimeout)
	}
	if notifierConfig.MaxRetryAttempts != defaultNotifierMaxRetryAttempts {
		t.Fatalf("expected default max retry attempts %d, got %d", defaultNotifierMaxRetryAttempts, notifierConfig.MaxRetryAttempts)
	}
	if notifierConfig.InitialRetryDelay != defaultNotifierInitialRetryDelay {
		t.Fatalf("expected default retry delay %s, got %s", defaultNotifierInitialRetryDelay, notifierConfig.InitialRetryDelay)
	}
	if notifierConfig.QueueClaimBatchSize != defaultNotifierQueueClaimBatchSize {
		t.Fatalf("expected default queue claim batch size %d, got %d", defaultNotifierQueueClaimBatchSize, notifierConfig.QueueClaimBatchSize)
	}
	if notifierConfig.QueuePollInterval != defaultNotifierQueuePollInterval {
		t.Fatalf("expected default queue poll interval %s, got %s", defaultNotifierQueuePollInterval, notifierConfig.QueuePollInterval)
	}
	if notifierConfig.SchedulerBufferMultiplier != defaultNotifierSchedulerBufferMultiplier {
		t.Fatalf("expected default scheduler buffer multiplier %d, got %d", defaultNotifierSchedulerBufferMultiplier, notifierConfig.SchedulerBufferMultiplier)
	}
	if notifierConfig.MetricsReportInterval != defaultNotifierMetricsReportInterval {
		t.Fatalf("expected default metrics interval %s, got %s", defaultNotifierMetricsReportInterval, notifierConfig.MetricsReportInterval)
	}
	if notifierConfig.ShutdownTimeout != defaultNotifierShutdownTimeout {
		t.Fatalf("expected default shutdown timeout %s, got %s", defaultNotifierShutdownTimeout, notifierConfig.ShutdownTimeout)
	}
	if notifierConfig.LogLevel != "INFO" {
		t.Fatalf("expected default log level INFO, got %s", notifierConfig.LogLevel)
	}
	if notifierConfig.RegistrationResolveQuery == "" || notifierConfig.RegistrationSnapshotQuery == "" {
		t.Fatalf("expected default registration queries to be populated")
	}
}

func TestLoadNotifierConfigReadsOverrides(t *testing.T) {
	// Input: notifier environment variables set to custom values.
	// Outcome: notifier config parses and returns the overrides.
	t.Setenv("NOTIFIER_HTTP_ADDRESS", ":29090")
	t.Setenv("NOTIFIER_WORKER_COUNT", "7")
	t.Setenv("NOTIFIER_REQUEST_TIMEOUT", "9s")
	t.Setenv("NOTIFIER_MAX_RETRY_ATTEMPTS", "6")
	t.Setenv("NOTIFIER_INITIAL_RETRY_DELAY", "3s")
	t.Setenv("NOTIFIER_QUEUE_CLAIM_BATCH_SIZE", "44")
	t.Setenv("NOTIFIER_QUEUE_POLL_INTERVAL", "150ms")
	t.Setenv("NOTIFIER_SCHEDULER_BUFFER_MULTIPLIER", "8")
	t.Setenv("NOTIFIER_METRICS_REPORT_INTERVAL", "4s")
	t.Setenv("NOTIFIER_SHUTDOWN_TIMEOUT", "11s")
	t.Setenv("NOTIFIER_LOG_LEVEL", "DEBUG")
	t.Setenv("NOTIFIER_POSTGRES_DSN", "postgres://custom")
	t.Setenv("NOTIFIER_REGISTRATION_RESOLVE_QUERY", "SELECT resolve")
	t.Setenv("NOTIFIER_REGISTRATION_SNAPSHOT_QUERY", "SELECT snapshot")

	notifierConfig, loadError := LoadNotifierConfig()
	if loadError != nil {
		t.Fatalf("load notifier config: %v", loadError)
	}

	if notifierConfig.HTTPAddress != ":29090" ||
		notifierConfig.WorkerCount != 7 ||
		notifierConfig.RequestTimeout != 9*time.Second ||
		notifierConfig.MaxRetryAttempts != 6 ||
		notifierConfig.InitialRetryDelay != 3*time.Second ||
		notifierConfig.QueueClaimBatchSize != 44 ||
		notifierConfig.QueuePollInterval != 150*time.Millisecond ||
		notifierConfig.SchedulerBufferMultiplier != 8 ||
		notifierConfig.MetricsReportInterval != 4*time.Second ||
		notifierConfig.ShutdownTimeout != 11*time.Second ||
		notifierConfig.LogLevel != "DEBUG" ||
		notifierConfig.PostgresConnection != "postgres://custom" ||
		notifierConfig.RegistrationResolveQuery != "SELECT resolve" ||
		notifierConfig.RegistrationSnapshotQuery != "SELECT snapshot" {
		t.Fatalf("unexpected notifier config: %#v", notifierConfig)
	}
}

func TestLoadNotifierConfigRejectsMissingPostgresDSN(t *testing.T) {
	// Input: notifier config with a blank PostgreSQL DSN.
	// Outcome: loading fails with a required-variable error.
	t.Setenv("NOTIFIER_POSTGRES_DSN", "   ")

	_, loadError := LoadNotifierConfig()
	if loadError == nil || loadError.Error() != "NOTIFIER_POSTGRES_DSN is required" {
		t.Fatalf("expected required DSN error, got %v", loadError)
	}
}

func TestLoadNotifierConfigRejectsParseFailures(t *testing.T) {
	testCases := []struct {
		name                string
		environmentVariable string
		value               string
	}{
		{
			name:                "input invalid worker count expects parse error",
			environmentVariable: "NOTIFIER_WORKER_COUNT",
			value:               "nope",
		},
		{
			name:                "input invalid max retry attempts expects parse error",
			environmentVariable: "NOTIFIER_MAX_RETRY_ATTEMPTS",
			value:               "nope",
		},
		{
			name:                "input invalid request timeout expects parse error",
			environmentVariable: "NOTIFIER_REQUEST_TIMEOUT",
			value:               "not-a-duration",
		},
		{
			name:                "input invalid initial retry delay expects parse error",
			environmentVariable: "NOTIFIER_INITIAL_RETRY_DELAY",
			value:               "not-a-duration",
		},
		{
			name:                "input invalid queue claim batch size expects parse error",
			environmentVariable: "NOTIFIER_QUEUE_CLAIM_BATCH_SIZE",
			value:               "nope",
		},
		{
			name:                "input invalid queue poll interval expects parse error",
			environmentVariable: "NOTIFIER_QUEUE_POLL_INTERVAL",
			value:               "not-a-duration",
		},
		{
			name:                "input invalid scheduler buffer multiplier expects parse error",
			environmentVariable: "NOTIFIER_SCHEDULER_BUFFER_MULTIPLIER",
			value:               "nope",
		},
		{
			name:                "input invalid metrics report interval expects parse error",
			environmentVariable: "NOTIFIER_METRICS_REPORT_INTERVAL",
			value:               "not-a-duration",
		},
		{
			name:                "input invalid shutdown timeout expects parse error",
			environmentVariable: "NOTIFIER_SHUTDOWN_TIMEOUT",
			value:               "not-a-duration",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("NOTIFIER_POSTGRES_DSN", "postgres://example")
			t.Setenv(testCase.environmentVariable, testCase.value)

			_, loadError := LoadNotifierConfig()
			if loadError == nil || !strings.Contains(loadError.Error(), "parse "+testCase.environmentVariable) {
				t.Fatalf("expected parse error for %s, got %v", testCase.environmentVariable, loadError)
			}
		})
	}
}

func TestLoadMockReceiverConfigUsesDefaultsAndRejectsInvalidDuration(t *testing.T) {
	// Input: receiver config loaded first with defaults and then with an invalid timeout value.
	// Outcome: default fields are returned first, and invalid duration input is rejected second.
	receiverConfig, loadError := LoadMockReceiverConfig()
	if loadError != nil {
		t.Fatalf("load receiver config: %v", loadError)
	}
	if receiverConfig.HTTPAddress != ":28082" ||
		receiverConfig.ShutdownTimeout != defaultReceiverShutdownTimeout ||
		receiverConfig.LogLevel != "INFO" {
		t.Fatalf("unexpected receiver config: %#v", receiverConfig)
	}

	t.Setenv("RECEIVER_SHUTDOWN_TIMEOUT", "bad-duration")
	_, loadError = LoadMockReceiverConfig()
	if loadError == nil || !strings.Contains(loadError.Error(), "parse RECEIVER_SHUTDOWN_TIMEOUT") {
		t.Fatalf("expected receiver parse error, got %v", loadError)
	}
}

func TestLoadMockGeneratorConfigUsesDefaultsAndOverrides(t *testing.T) {
	// Input: generator config loaded once with defaults and once with custom overrides.
	// Outcome: defaults are applied first and custom values are parsed on the second load.
	generatorConfig, loadError := LoadMockGeneratorConfig()
	if loadError != nil {
		t.Fatalf("load default generator config: %v", loadError)
	}
	if generatorConfig.HTTPAddress != ":28081" ||
		generatorConfig.NotifierBaseURL != "http://localhost:28080" ||
		generatorConfig.DefaultCustomerCount != defaultGeneratorCustomerCount ||
		generatorConfig.HTTPRequestTimeout != defaultGeneratorHTTPTimeout ||
		generatorConfig.ShutdownTimeout != defaultGeneratorShutdownTimeout ||
		generatorConfig.LogLevel != "INFO" {
		t.Fatalf("unexpected default generator config: %#v", generatorConfig)
	}

	t.Setenv("GENERATOR_HTTP_ADDRESS", ":30001")
	t.Setenv("GENERATOR_NOTIFIER_BASE_URL", "http://example.test/")
	t.Setenv("GENERATOR_DEFAULT_CUSTOMER_COUNT", "9")
	t.Setenv("GENERATOR_RANDOM_SEED", "123")
	t.Setenv("GENERATOR_HTTP_REQUEST_TIMEOUT", "8s")
	t.Setenv("GENERATOR_SHUTDOWN_TIMEOUT", "6s")
	t.Setenv("GENERATOR_LOG_LEVEL", "WARN")

	generatorConfig, loadError = LoadMockGeneratorConfig()
	if loadError != nil {
		t.Fatalf("load override generator config: %v", loadError)
	}
	if generatorConfig.HTTPAddress != ":30001" ||
		generatorConfig.NotifierBaseURL != "http://example.test" ||
		generatorConfig.DefaultCustomerCount != 9 ||
		generatorConfig.RandomSeed != 123 ||
		generatorConfig.HTTPRequestTimeout != 8*time.Second ||
		generatorConfig.ShutdownTimeout != 6*time.Second ||
		generatorConfig.LogLevel != "WARN" {
		t.Fatalf("unexpected override generator config: %#v", generatorConfig)
	}
}

func TestLoadMockGeneratorConfigRejectsParseFailures(t *testing.T) {
	testCases := []struct {
		name                string
		environmentVariable string
		value               string
	}{
		{
			name:                "input invalid customer count expects parse error",
			environmentVariable: "GENERATOR_DEFAULT_CUSTOMER_COUNT",
			value:               "bad-int",
		},
		{
			name:                "input invalid random seed expects parse error",
			environmentVariable: "GENERATOR_RANDOM_SEED",
			value:               "bad-int64",
		},
		{
			name:                "input invalid request timeout expects parse error",
			environmentVariable: "GENERATOR_HTTP_REQUEST_TIMEOUT",
			value:               "bad-duration",
		},
		{
			name:                "input invalid shutdown timeout expects parse error",
			environmentVariable: "GENERATOR_SHUTDOWN_TIMEOUT",
			value:               "bad-duration",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.environmentVariable, testCase.value)

			_, loadError := LoadMockGeneratorConfig()
			if loadError == nil || !strings.Contains(loadError.Error(), "parse "+testCase.environmentVariable) {
				t.Fatalf("expected parse error for %s, got %v", testCase.environmentVariable, loadError)
			}
		})
	}
}
