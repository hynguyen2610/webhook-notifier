package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/logging"
	"webhook-notifier/internal/notifier"
)

func main() {
	notifierConfig, configurationError := config.LoadNotifierConfig()
	if configurationError != nil {
		log.Fatalf("load notifier config: %v", configurationError)
	}

	logger := logging.NewLogger(notifierConfig.LogLevel)
	application := notifier.NewApplication(notifierConfig, logger)

	requestContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	if runError := application.Run(requestContext); runError != nil {
		logger.Error("notifier stopped with error", "error", runError)
		log.Fatalf("run notifier: %v", runError)
	}
}
