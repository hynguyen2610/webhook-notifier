package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type NotifierConfig struct {
	HTTPAddress               string
	WorkerCount               int
	RequestTimeout            time.Duration
	MaxRetryAttempts          int
	InitialRetryDelay         time.Duration
	LogLevel                  string
	KafkaBrokers              []string
	KafkaTopic                string
	KafkaConsumerGroup        string
	KafkaDLQTopic             string
	PostgresConnection        string
	RegistrationResolveQuery  string
	RegistrationSnapshotQuery string
}

type MockReceiverConfig struct {
	HTTPAddress string
	LogLevel    string
}

type MockGeneratorConfig struct {
	HTTPAddress          string
	NotifierBaseURL      string
	DefaultCustomerCount int
	RandomSeed           int64
	LogLevel             string
	KafkaBrokers         []string
	KafkaTopic           string
}

func LoadNotifierConfig() (NotifierConfig, error) {
	workerCount, workerCountError := parseIntEnvironment("NOTIFIER_WORKER_COUNT", 4)
	if workerCountError != nil {
		return NotifierConfig{}, workerCountError
	}

	maxRetryAttempts, retryError := parseIntEnvironment("NOTIFIER_MAX_RETRY_ATTEMPTS", 3)
	if retryError != nil {
		return NotifierConfig{}, retryError
	}

	requestTimeout, requestTimeoutError := parseDurationEnvironment("NOTIFIER_REQUEST_TIMEOUT", 5*time.Second)
	if requestTimeoutError != nil {
		return NotifierConfig{}, requestTimeoutError
	}

	initialRetryDelay, retryDelayError := parseDurationEnvironment("NOTIFIER_INITIAL_RETRY_DELAY", time.Second)
	if retryDelayError != nil {
		return NotifierConfig{}, retryDelayError
	}

	postgresConnection := readEnvironment("NOTIFIER_POSTGRES_DSN", "")
	if strings.TrimSpace(postgresConnection) == "" {
		return NotifierConfig{}, errors.New("NOTIFIER_POSTGRES_DSN is required")
	}

	return NotifierConfig{
		HTTPAddress:               readEnvironment("NOTIFIER_HTTP_ADDRESS", ":8080"),
		WorkerCount:               workerCount,
		RequestTimeout:            requestTimeout,
		MaxRetryAttempts:          maxRetryAttempts,
		InitialRetryDelay:         initialRetryDelay,
		LogLevel:                  readEnvironment("NOTIFIER_LOG_LEVEL", "INFO"),
		KafkaBrokers:              splitCommaSeparatedValues(readEnvironment("NOTIFIER_KAFKA_BROKERS", "")),
		KafkaTopic:                readEnvironment("NOTIFIER_KAFKA_TOPIC", "subscriber-events"),
		KafkaConsumerGroup:        readEnvironment("NOTIFIER_KAFKA_CONSUMER_GROUP", "webhook-notifier"),
		KafkaDLQTopic:             readEnvironment("NOTIFIER_KAFKA_DLQ_TOPIC", "subscriber-events-dlq"),
		PostgresConnection:        postgresConnection,
		RegistrationResolveQuery:  readEnvironment("NOTIFIER_REGISTRATION_RESOLVE_QUERY", "SELECT webhook_url FROM webhook_registrations WHERE customer_id = $1 AND is_active = TRUE ORDER BY webhook_url"),
		RegistrationSnapshotQuery: readEnvironment("NOTIFIER_REGISTRATION_SNAPSHOT_QUERY", "SELECT customer_id, webhook_url FROM webhook_registrations WHERE is_active = TRUE ORDER BY customer_id, webhook_url"),
	}, nil
}

func LoadMockReceiverConfig() (MockReceiverConfig, error) {
	return MockReceiverConfig{
		HTTPAddress: readEnvironment("RECEIVER_HTTP_ADDRESS", ":8082"),
		LogLevel:    readEnvironment("RECEIVER_LOG_LEVEL", "INFO"),
	}, nil
}

func LoadMockGeneratorConfig() (MockGeneratorConfig, error) {
	defaultCustomerCount, customerCountError := parseIntEnvironment("GENERATOR_DEFAULT_CUSTOMER_COUNT", 5)
	if customerCountError != nil {
		return MockGeneratorConfig{}, customerCountError
	}

	randomSeed, randomSeedError := parseInt64Environment("GENERATOR_RANDOM_SEED", time.Now().UnixNano())
	if randomSeedError != nil {
		return MockGeneratorConfig{}, randomSeedError
	}

	return MockGeneratorConfig{
		HTTPAddress:          readEnvironment("GENERATOR_HTTP_ADDRESS", ":8081"),
		NotifierBaseURL:      strings.TrimRight(readEnvironment("GENERATOR_NOTIFIER_BASE_URL", "http://localhost:8080"), "/"),
		DefaultCustomerCount: defaultCustomerCount,
		RandomSeed:           randomSeed,
		LogLevel:             readEnvironment("GENERATOR_LOG_LEVEL", "INFO"),
		KafkaBrokers:         splitCommaSeparatedValues(readEnvironment("GENERATOR_KAFKA_BROKERS", "")),
		KafkaTopic:           readEnvironment("GENERATOR_KAFKA_TOPIC", "subscriber-events"),
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

func splitCommaSeparatedValues(rawValue string) []string {
	if strings.TrimSpace(rawValue) == "" {
		return nil
	}

	rawParts := strings.Split(rawValue, ",")
	parts := make([]string, 0, len(rawParts))
	for _, rawPart := range rawParts {
		trimmedPart := strings.TrimSpace(rawPart)
		if trimmedPart != "" {
			parts = append(parts, trimmedPart)
		}
	}

	return parts
}
