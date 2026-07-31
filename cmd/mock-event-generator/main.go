package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/logging"
	"webhook-notifier/internal/mockgenerator"
)

func main() {
	generatorConfig, configurationError := config.LoadMockGeneratorConfig()
	if configurationError != nil {
		log.Fatalf("load generator config: %v", configurationError)
	}

	logger := logging.NewLogger(generatorConfig.LogLevel)
	application := mockgenerator.NewApplication(generatorConfig, logger)

	requestContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	if runError := application.Run(requestContext); runError != nil {
		logger.Error("generator stopped with error", "error", runError)
		log.Fatalf("run generator: %v", runError)
	}
}
