package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultNotifierWorkerCount               = 4
	defaultNotifierRequestTimeout            = 5 * time.Second
	defaultNotifierMaxRetryAttempts          = 3
	defaultNotifierInitialRetryDelay         = time.Second
	defaultNotifierQueueClaimBatchSize       = 32
	defaultNotifierQueuePollInterval         = 250 * time.Millisecond
	defaultNotifierSchedulerBufferMultiplier = 4
	defaultNotifierMetricsReportInterval     = 2 * time.Second
	defaultNotifierShutdownTimeout           = 5 * time.Second

	defaultReceiverShutdownTimeout = 5 * time.Second

	defaultGeneratorCustomerCount   = 5
	defaultGeneratorHTTPTimeout     = 15 * time.Second
	defaultGeneratorShutdownTimeout = 5 * time.Second
)

type NotifierConfig struct {
	HTTPAddress               string
	WorkerCount               int
	RequestTimeout            time.Duration
	MaxRetryAttempts          int
	InitialRetryDelay         time.Duration
	QueueClaimBatchSize       int
	QueuePollInterval         time.Duration
	SchedulerBufferMultiplier int
	MetricsReportInterval     time.Duration
	ShutdownTimeout           time.Duration
	LogLevel                  string
	PostgresConnection        string
	RegistrationResolveQuery  string
	RegistrationSnapshotQuery string
}

type MockReceiverConfig struct {
	HTTPAddress     string
	ShutdownTimeout time.Duration
	LogLevel        string
}

type MockGeneratorConfig struct {
	HTTPAddress          string
	NotifierBaseURL      string
	DefaultCustomerCount int
	RandomSeed           int64
	HTTPRequestTimeout   time.Duration
	ShutdownTimeout      time.Duration
	LogLevel             string
}

func LoadNotifierConfig() (NotifierConfig, error) {
	workerCount, workerCountError := parseIntEnvironment("NOTIFIER_WORKER_COUNT", defaultNotifierWorkerCount)
	if workerCountError != nil {
		return NotifierConfig{}, workerCountError
	}

	maxRetryAttempts, retryError := parseIntEnvironment("NOTIFIER_MAX_RETRY_ATTEMPTS", defaultNotifierMaxRetryAttempts)
	if retryError != nil {
		return NotifierConfig{}, retryError
	}

	requestTimeout, requestTimeoutError := parseDurationEnvironment("NOTIFIER_REQUEST_TIMEOUT", defaultNotifierRequestTimeout)
	if requestTimeoutError != nil {
		return NotifierConfig{}, requestTimeoutError
	}

	initialRetryDelay, retryDelayError := parseDurationEnvironment("NOTIFIER_INITIAL_RETRY_DELAY", defaultNotifierInitialRetryDelay)
	if retryDelayError != nil {
		return NotifierConfig{}, retryDelayError
	}

	queueClaimBatchSize, queueBatchError := parseIntEnvironment("NOTIFIER_QUEUE_CLAIM_BATCH_SIZE", defaultNotifierQueueClaimBatchSize)
	if queueBatchError != nil {
		return NotifierConfig{}, queueBatchError
	}

	queuePollInterval, queuePollError := parseDurationEnvironment("NOTIFIER_QUEUE_POLL_INTERVAL", defaultNotifierQueuePollInterval)
	if queuePollError != nil {
		return NotifierConfig{}, queuePollError
	}

	schedulerBufferMultiplier, schedulerBufferMultiplierError := parseIntEnvironment("NOTIFIER_SCHEDULER_BUFFER_MULTIPLIER", defaultNotifierSchedulerBufferMultiplier)
	if schedulerBufferMultiplierError != nil {
		return NotifierConfig{}, schedulerBufferMultiplierError
	}

	metricsReportInterval, metricsReportIntervalError := parseDurationEnvironment("NOTIFIER_METRICS_REPORT_INTERVAL", defaultNotifierMetricsReportInterval)
	if metricsReportIntervalError != nil {
		return NotifierConfig{}, metricsReportIntervalError
	}

	shutdownTimeout, shutdownTimeoutError := parseDurationEnvironment("NOTIFIER_SHUTDOWN_TIMEOUT", defaultNotifierShutdownTimeout)
	if shutdownTimeoutError != nil {
		return NotifierConfig{}, shutdownTimeoutError
	}

	postgresConnection := readEnvironment("NOTIFIER_POSTGRES_DSN", "")
	if strings.TrimSpace(postgresConnection) == "" {
		return NotifierConfig{}, errors.New("NOTIFIER_POSTGRES_DSN is required")
	}

	return NotifierConfig{
		HTTPAddress:               readEnvironment("NOTIFIER_HTTP_ADDRESS", ":28080"),
		WorkerCount:               workerCount,
		RequestTimeout:            requestTimeout,
		MaxRetryAttempts:          maxRetryAttempts,
		InitialRetryDelay:         initialRetryDelay,
		QueueClaimBatchSize:       queueClaimBatchSize,
		QueuePollInterval:         queuePollInterval,
		SchedulerBufferMultiplier: schedulerBufferMultiplier,
		MetricsReportInterval:     metricsReportInterval,
		ShutdownTimeout:           shutdownTimeout,
		LogLevel:                  readEnvironment("NOTIFIER_LOG_LEVEL", "INFO"),
		PostgresConnection:        postgresConnection,
		RegistrationResolveQuery:  readEnvironment("NOTIFIER_REGISTRATION_RESOLVE_QUERY", "SELECT webhook_url FROM webhook_registrations WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url"),
		RegistrationSnapshotQuery: readEnvironment("NOTIFIER_REGISTRATION_SNAPSHOT_QUERY", "SELECT customer_id, webhook_url FROM webhook_registrations WHERE is_active = TRUE ORDER BY customer_id, webhook_url"),
	}, nil
}

func LoadMockReceiverConfig() (MockReceiverConfig, error) {
	shutdownTimeout, shutdownTimeoutError := parseDurationEnvironment("RECEIVER_SHUTDOWN_TIMEOUT", defaultReceiverShutdownTimeout)
	if shutdownTimeoutError != nil {
		return MockReceiverConfig{}, shutdownTimeoutError
	}

	return MockReceiverConfig{
		HTTPAddress:     readEnvironment("RECEIVER_HTTP_ADDRESS", ":28082"),
		ShutdownTimeout: shutdownTimeout,
		LogLevel:        readEnvironment("RECEIVER_LOG_LEVEL", "INFO"),
	}, nil
}

func LoadMockGeneratorConfig() (MockGeneratorConfig, error) {
	defaultCustomerCount, customerCountError := parseIntEnvironment("GENERATOR_DEFAULT_CUSTOMER_COUNT", defaultGeneratorCustomerCount)
	if customerCountError != nil {
		return MockGeneratorConfig{}, customerCountError
	}

	randomSeed, randomSeedError := parseInt64Environment("GENERATOR_RANDOM_SEED", time.Now().UnixNano())
	if randomSeedError != nil {
		return MockGeneratorConfig{}, randomSeedError
	}

	httpRequestTimeout, httpRequestTimeoutError := parseDurationEnvironment("GENERATOR_HTTP_REQUEST_TIMEOUT", defaultGeneratorHTTPTimeout)
	if httpRequestTimeoutError != nil {
		return MockGeneratorConfig{}, httpRequestTimeoutError
	}

	shutdownTimeout, shutdownTimeoutError := parseDurationEnvironment("GENERATOR_SHUTDOWN_TIMEOUT", defaultGeneratorShutdownTimeout)
	if shutdownTimeoutError != nil {
		return MockGeneratorConfig{}, shutdownTimeoutError
	}

	return MockGeneratorConfig{
		HTTPAddress:          readEnvironment("GENERATOR_HTTP_ADDRESS", ":28081"),
		NotifierBaseURL:      strings.TrimRight(readEnvironment("GENERATOR_NOTIFIER_BASE_URL", "http://localhost:28080"), "/"),
		DefaultCustomerCount: defaultCustomerCount,
		RandomSeed:           randomSeed,
		HTTPRequestTimeout:   httpRequestTimeout,
		ShutdownTimeout:      shutdownTimeout,
		LogLevel:             readEnvironment("GENERATOR_LOG_LEVEL", "INFO"),
	}, nil
}

func readEnvironment(environmentVariable string, defaultValue string) string {
	value, found := os.LookupEnv(environmentVariable)
	if !found || strings.TrimSpace(value) == "" {
		return defaultValue
	}

	return value
}

func parseIntEnvironment(environmentVariable string, defaultValue int) (int, error) {
	rawValue := readEnvironment(environmentVariable, strconv.Itoa(defaultValue))
	parsedValue, parseError := strconv.Atoi(rawValue)
	if parseError != nil {
		return 0, fmt.Errorf("parse %s: %w", environmentVariable, parseError)
	}

	return parsedValue, nil
}

func parseInt64Environment(environmentVariable string, defaultValue int64) (int64, error) {
	rawValue := readEnvironment(environmentVariable, strconv.FormatInt(defaultValue, 10))
	parsedValue, parseError := strconv.ParseInt(rawValue, 10, 64)
	if parseError != nil {
		return 0, fmt.Errorf("parse %s: %w", environmentVariable, parseError)
	}

	return parsedValue, nil
}

func parseDurationEnvironment(environmentVariable string, defaultValue time.Duration) (time.Duration, error) {
	rawValue := readEnvironment(environmentVariable, defaultValue.String())
	parsedValue, parseError := time.ParseDuration(rawValue)
	if parseError != nil {
		return 0, fmt.Errorf("parse %s: %w", environmentVariable, parseError)
	}

	return parsedValue, nil
}
