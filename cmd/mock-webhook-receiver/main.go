package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"webhook-notifier/internal/config"
	"webhook-notifier/internal/logging"
	"webhook-notifier/internal/mockreceiver"
)

func main() {
	receiverConfig, configurationError := config.LoadMockReceiverConfig()
	if configurationError != nil {
		log.Fatalf("load receiver config: %v", configurationError)
	}

	logger := logging.NewLogger(receiverConfig.LogLevel)
	application := mockreceiver.NewApplication(receiverConfig, logger)

	requestContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	if runError := application.Run(requestContext); runError != nil {
		logger.Error("receiver stopped with error", "error", runError)
		log.Fatalf("run receiver: %v", runError)
	}
}
