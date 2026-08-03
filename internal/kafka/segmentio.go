package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	segmentio "github.com/segmentio/kafka-go"

	"webhook-notifier/internal/events"
)

type EventPublisher struct {
	writer *segmentio.Writer
}

type DeadLetterPublisher struct {
	writer *segmentio.Writer
}

type EventConsumer struct {
	reader *segmentio.Reader
}

type StaticResolver struct {
	hostOverrides map[string]string
}

func NewEventPublisher(brokers []string, hostOverrides map[string]string, topic string) *EventPublisher {
	return &EventPublisher{
		writer: segmentio.NewWriter(segmentio.WriterConfig{
			Brokers:      brokers,
			Dialer:       newDialer(hostOverrides),
			Topic:        topic,
			Balancer:     &segmentio.LeastBytes{},
			RequiredAcks: int(segmentio.RequireOne),
		}),
	}
}

func NewDeadLetterPublisher(brokers []string, hostOverrides map[string]string, topic string) *DeadLetterPublisher {
	return &DeadLetterPublisher{
		writer: segmentio.NewWriter(segmentio.WriterConfig{
			Brokers:      brokers,
			Dialer:       newDialer(hostOverrides),
			Topic:        topic,
			Balancer:     &segmentio.LeastBytes{},
			RequiredAcks: int(segmentio.RequireOne),
		}),
	}
}

func NewEventConsumer(brokers []string, hostOverrides map[string]string, topic string, consumerGroup string) *EventConsumer {
	return &EventConsumer{
		reader: segmentio.NewReader(segmentio.ReaderConfig{
			Brokers:     brokers,
			Dialer:      newDialer(hostOverrides),
			Topic:       topic,
			GroupID:     consumerGroup,
			MinBytes:    1,
			MaxBytes:    10e6,
			StartOffset: segmentio.FirstOffset,
			MaxWait:     2 * time.Second,
		}),
	}
}

func (publisher *EventPublisher) Publish(subscriberEvents []events.SubscriberEvent) error {
	requestContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messages := make([]segmentio.Message, 0, len(subscriberEvents))
	for _, subscriberEvent := range subscriberEvents {
		value, marshalError := json.Marshal(subscriberEvent)
		if marshalError != nil {
			return marshalError
		}

		messages = append(messages, segmentio.Message{
			Key:   []byte(subscriberEvent.CustomerID),
			Value: value,
			Time:  time.Now().UTC(),
		})
	}

	return publisher.writer.WriteMessages(requestContext, messages...)
}

func (publisher *DeadLetterPublisher) Publish(deadLetterMessage events.DeadLetterMessage) error {
	requestContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	value, marshalError := json.Marshal(deadLetterMessage)
	if marshalError != nil {
		return marshalError
	}

	message := segmentio.Message{
		Key:   []byte(deadLetterMessage.Job.Event.CustomerID),
		Value: value,
		Time:  time.Now().UTC(),
	}

	return publisher.writer.WriteMessages(requestContext, message)
}

func (consumer *EventConsumer) Start(requestContext context.Context, handler func(events.SubscriberEvent) error) error {
	defer consumer.reader.Close()

	for {
		message, readError := consumer.reader.FetchMessage(requestContext)
		if readError != nil {
			if errors.Is(readError, context.Canceled) {
				return nil
			}
			return readError
		}

		var subscriberEvent events.SubscriberEvent
		if unmarshalError := json.Unmarshal(message.Value, &subscriberEvent); unmarshalError != nil {
			return fmt.Errorf("decode kafka message: %w", unmarshalError)
		}

		if handlerError := handler(subscriberEvent); handlerError != nil {
			return handlerError
		}

		if commitError := consumer.reader.CommitMessages(requestContext, message); commitError != nil {
			if errors.Is(commitError, context.Canceled) || requestContext.Err() != nil {
				return nil
			}
			return fmt.Errorf("commit kafka message: %w", commitError)
		}
	}
}

func (publisher *EventPublisher) Close() error {
	return publisher.writer.Close()
}

func (publisher *DeadLetterPublisher) Close() error {
	return publisher.writer.Close()
}

func newDialer(hostOverrides map[string]string) *segmentio.Dialer {
	if len(hostOverrides) == 0 {
		return nil
	}

	return &segmentio.Dialer{
		Timeout:  10 * time.Second,
		Resolver: StaticResolver{hostOverrides: hostOverrides},
	}
}

func (resolver StaticResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	if overrideAddress, found := resolver.hostOverrides[host]; found {
		return []string{overrideAddress}, nil
	}

	return net.DefaultResolver.LookupHost(context.Background(), host)
}
