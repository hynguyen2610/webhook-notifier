package events

import "testing"

func TestSupportedSubscriberEventTypesExposeExpectedValues(t *testing.T) {
	// Input: the exported subscriber event type constants and supported-types slice.
	// Outcome: the constants keep their expected values and the supported slice lists all of them in order.
	if SubscriberCreatedEventType != "subscriber.created" {
		t.Fatalf("unexpected created event type %q", SubscriberCreatedEventType)
	}
	if SubscriberAddedToSegmentEventType != "subscriber.added_to_segment" {
		t.Fatalf("unexpected added-to-segment event type %q", SubscriberAddedToSegmentEventType)
	}
	if SubscriberUnsubscribedEventType != "subscriber.unsubscribed" {
		t.Fatalf("unexpected unsubscribed event type %q", SubscriberUnsubscribedEventType)
	}

	expectedTypes := []string{
		SubscriberCreatedEventType,
		SubscriberAddedToSegmentEventType,
		SubscriberUnsubscribedEventType,
	}
	if len(SupportedSubscriberEventTypes) != len(expectedTypes) {
		t.Fatalf("expected %d supported types, got %#v", len(expectedTypes), SupportedSubscriberEventTypes)
	}
	for index := range expectedTypes {
		if SupportedSubscriberEventTypes[index] != expectedTypes[index] {
			t.Fatalf("unexpected supported types %#v", SupportedSubscriberEventTypes)
		}
	}
}
