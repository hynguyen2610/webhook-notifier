package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	segmentio "github.com/segmentio/kafka-go"

	"webhook-notifier/internal/events"
)

func TestKafkaIntegrationPublishesAndConsumesSubscriberEvent(t *testing.T) {
	// Input: one subscriber event published to a real Kafka topic with a consumer group already subscribed.
	// Outcome: the consumer reads the live Kafka message, decodes it, and passes the original event to the handler.
	kafkaBrokers := integrationKafkaBrokers(t)
	topicName := fmt.Sprintf("subscriber-events-it-%d", time.Now().UnixNano())
	consumerGroup := fmt.Sprintf("webhook-notifier-it-%d", time.Now().UnixNano())
	createKafkaTopic(t, kafkaBrokers, topicName, 1)

	eventPublisher := NewEventPublisher(kafkaBrokers, nil, topicName)
	defer eventPublisher.Close()

	eventConsumer := NewEventConsumer(kafkaBrokers, nil, topicName, consumerGroup)

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelRequest()

	receivedEvents := make(chan events.SubscriberEvent, 1)
	consumerErrors := make(chan error, 1)
	go func() {
		consumerErrors <- eventConsumer.Start(requestContext, func(subscriberEvent events.SubscriberEvent) error {
			receivedEvents <- subscriberEvent
			return nil
		})
	}()

	time.Sleep(2 * time.Second)

	expectedEvent := events.SubscriberEvent{
		EventID:      fmt.Sprintf("event-%d", time.Now().UnixNano()),
		CustomerID:   "customer-a",
		SubscriberID: "subscriber-001",
		EventType:    "subscriber.created",
		OccurredAt:   time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC),
	}

	if publishError := eventPublisher.Publish([]events.SubscriberEvent{expectedEvent}); publishError != nil {
		t.Fatalf("publish event: %v", publishError)
	}

	select {
	case receivedEvent := <-receivedEvents:
		if receivedEvent != expectedEvent {
			t.Fatalf("unexpected received event: got %#v want %#v", receivedEvent, expectedEvent)
		}
	case consumerError := <-consumerErrors:
		if consumerError != nil {
			t.Fatalf("consume event: %v", consumerError)
		}
		t.Fatal("consumer exited before receiving event")
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for consumed event")
	}

	cancelRequest()
	if consumerError := <-consumerErrors; consumerError != nil {
		t.Fatalf("consumer returned error: %v", consumerError)
	}
}

func TestKafkaIntegrationPublishesDeadLetterMessage(t *testing.T) {
	// Input: one dead-letter message published to a real Kafka DLQ topic.
	// Outcome: a live Kafka reader receives the DLQ payload with the same event ID and failure reason.
	kafkaBrokers := integrationKafkaBrokers(t)
	topicName := fmt.Sprintf("subscriber-events-dlq-it-%d", time.Now().UnixNano())
	createKafkaTopic(t, kafkaBrokers, topicName, 1)

	deadLetterPublisher := NewDeadLetterPublisher(kafkaBrokers, nil, topicName)
	defer deadLetterPublisher.Close()

	deadLetterMessage := events.DeadLetterMessage{
		Job: events.DeliveryJob{
			Event: events.SubscriberEvent{
				EventID:      fmt.Sprintf("dead-letter-event-%d", time.Now().UnixNano()),
				CustomerID:   "customer-a",
				SubscriberID: "subscriber-001",
				EventType:    "subscriber.updated",
				OccurredAt:   time.Date(2026, time.August, 3, 10, 5, 0, 0, time.UTC),
			},
			WebhookURL: "https://example.com/webhook/customer-a",
			Attempt:    2,
			EnqueuedAt: time.Date(2026, time.August, 3, 10, 4, 0, 0, time.UTC),
		},
		FailureReason: "webhook returned status 500",
		ExhaustedAt:   time.Date(2026, time.August, 3, 10, 6, 0, 0, time.UTC),
	}

	if publishError := deadLetterPublisher.Publish(deadLetterMessage); publishError != nil {
		t.Fatalf("publish dead letter message: %v", publishError)
	}

	kafkaReader := segmentio.NewReader(segmentio.ReaderConfig{
		Brokers:   kafkaBrokers,
		Topic:     topicName,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
		MaxWait:   2 * time.Second,
	})
	defer kafkaReader.Close()

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelRequest()

	message, readError := kafkaReader.ReadMessage(requestContext)
	if readError != nil {
		t.Fatalf("read dead letter message: %v", readError)
	}

	var receivedDeadLetter events.DeadLetterMessage
	if unmarshalError := json.Unmarshal(message.Value, &receivedDeadLetter); unmarshalError != nil {
		t.Fatalf("decode dead letter message: %v", unmarshalError)
	}

	if receivedDeadLetter.Job.Event.EventID != deadLetterMessage.Job.Event.EventID {
		t.Fatalf("unexpected DLQ event ID: got %s want %s", receivedDeadLetter.Job.Event.EventID, deadLetterMessage.Job.Event.EventID)
	}
	if receivedDeadLetter.FailureReason != deadLetterMessage.FailureReason {
		t.Fatalf("unexpected DLQ failure reason: got %s want %s", receivedDeadLetter.FailureReason, deadLetterMessage.FailureReason)
	}
}

func integrationKafkaBrokers(t *testing.T) []string {
	t.Helper()

	rawKafkaBrokers := os.Getenv("TEST_KAFKA_BROKERS")
	if strings.TrimSpace(rawKafkaBrokers) == "" {
		t.Skip("TEST_KAFKA_BROKERS is not set")
	}

	kafkaBrokers := make([]string, 0)
	for _, rawKafkaBroker := range strings.Split(rawKafkaBrokers, ",") {
		trimmedKafkaBroker := strings.TrimSpace(rawKafkaBroker)
		if trimmedKafkaBroker != "" {
			kafkaBrokers = append(kafkaBrokers, trimmedKafkaBroker)
		}
	}

	if len(kafkaBrokers) == 0 {
		t.Skip("TEST_KAFKA_BROKERS does not contain usable brokers")
	}

	return kafkaBrokers
}

func createKafkaTopic(t *testing.T, kafkaBrokers []string, topicName string, partitionCount int) {
	t.Helper()

	brokerHost, brokerPort, splitError := net.SplitHostPort(kafkaBrokers[0])
	if splitError != nil {
		t.Fatalf("split kafka broker host and port: %v", splitError)
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelRequest()

	brokerConnection, dialError := segmentio.DialContext(requestContext, "tcp", net.JoinHostPort(brokerHost, brokerPort))
	if dialError != nil {
		t.Fatalf("dial kafka broker: %v", dialError)
	}
	defer brokerConnection.Close()

	controller, controllerError := brokerConnection.Controller()
	if controllerError != nil {
		t.Fatalf("lookup kafka controller: %v", controllerError)
	}

	controllerConnection, controllerDialError := segmentio.DialContext(
		requestContext,
		"tcp",
		net.JoinHostPort(controller.Host, fmt.Sprintf("%d", controller.Port)),
	)
	if controllerDialError != nil {
		t.Fatalf("dial kafka controller: %v", controllerDialError)
	}
	defer controllerConnection.Close()

	createTopicsError := controllerConnection.CreateTopics(segmentio.TopicConfig{
		Topic:             topicName,
		NumPartitions:     partitionCount,
		ReplicationFactor: 1,
	})
	if createTopicsError != nil && !strings.Contains(createTopicsError.Error(), "already exists") {
		t.Fatalf("create kafka topic %s: %v", topicName, createTopicsError)
	}
}
