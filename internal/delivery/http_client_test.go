package delivery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webhook-notifier/internal/events"
)

func TestDeliverReturnsSuccessResultAndHeadersForTwoHundredResponse(t *testing.T) {
	// Input: one delivery job sent to a healthy webhook endpoint.
	// Outcome: the client posts JSON with trace headers and reports a successful delivery result.
	var requestBody string
	var requestHeaders http.Header
	webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			t.Fatalf("read request body: %v", readError)
		}
		requestBody = string(bodyBytes)
		requestHeaders = request.Header.Clone()
		responseWriter.WriteHeader(http.StatusOK)
		_, _ = responseWriter.Write([]byte("delivered"))
	}))
	defer webhookServer.Close()

	client := NewHTTPClient(2 * time.Second)
	deliveryResult := client.Deliver(context.Background(), newDeliveryJob(webhookServer.URL))

	if deliveryResult.StatusCode != http.StatusOK || deliveryResult.ShouldRetry || deliveryResult.FailureReason != "" {
		t.Fatalf("unexpected delivery result: %#v", deliveryResult)
	}
	if !strings.Contains(requestBody, `"eventId":"event-1"`) {
		t.Fatalf("expected JSON payload with event ID, got %s", requestBody)
	}
	if requestHeaders.Get("Content-Type") != "application/json" ||
		requestHeaders.Get("X-Event-ID") != "event-1" ||
		requestHeaders.Get("X-Customer-ID") != "customer-a" ||
		requestHeaders.Get("X-Retry-Attempt") != "2" ||
		requestHeaders.Get("X-Trace-ID") != "trace-1" {
		t.Fatalf("unexpected request headers: %#v", requestHeaders)
	}
}

func TestDeliverReturnsMarshalFailureForUnserializableTimeValue(t *testing.T) {
	// Input: one delivery job whose occurredAt timestamp cannot be marshaled to JSON.
	// Outcome: the client returns a marshal failure without attempting the HTTP request.
	job := newDeliveryJob("https://example.com/webhook")
	job.Event.OccurredAt = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

	deliveryResult := NewHTTPClient(time.Second).Deliver(context.Background(), job)

	if !strings.Contains(deliveryResult.FailureReason, "marshal payload") {
		t.Fatalf("expected marshal failure, got %#v", deliveryResult)
	}
	if deliveryResult.StatusCode != 0 || deliveryResult.ShouldRetry {
		t.Fatalf("expected no status code and no retry, got %#v", deliveryResult)
	}
}

func TestDeliverReturnsRequestCreationFailureForInvalidWebhookURL(t *testing.T) {
	// Input: one delivery job with an invalid webhook URL.
	// Outcome: the client returns a request-creation error before any network call is made.
	job := newDeliveryJob("://bad-url")

	deliveryResult := NewHTTPClient(time.Second).Deliver(context.Background(), job)

	if !strings.Contains(deliveryResult.FailureReason, "create request") {
		t.Fatalf("expected request creation failure, got %#v", deliveryResult)
	}
	if deliveryResult.StatusCode != 0 || deliveryResult.ShouldRetry {
		t.Fatalf("expected no status code and no retry, got %#v", deliveryResult)
	}
}

func TestDeliverReturnsRetryableTransportFailureForTimeoutLikeError(t *testing.T) {
	// Input: one delivery job whose HTTP transport returns a timeout-style network error.
	// Outcome: the client marks the delivery as retryable and preserves the transport error text.
	client := &HTTPClient{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, timeoutNetError{message: "request timeout"}
			}),
		},
	}

	deliveryResult := client.Deliver(context.Background(), newDeliveryJob("https://example.com/webhook"))

	if !deliveryResult.ShouldRetry {
		t.Fatalf("expected retryable transport error, got %#v", deliveryResult)
	}
	if !strings.Contains(deliveryResult.FailureReason, "request timeout") {
		t.Fatalf("expected transport failure reason, got %#v", deliveryResult)
	}
}

func TestDeliverMapsRetryableAndNonRetryableStatusCodes(t *testing.T) {
	testCases := []struct {
		name              string
		statusCode        int
		expectedRetryable bool
		expectedBodyText  string
	}{
		{
			name:              "input http 429 expects retryable failure",
			statusCode:        http.StatusTooManyRequests,
			expectedRetryable: true,
			expectedBodyText:  "rate limited",
		},
		{
			name:              "input http 500 expects retryable failure",
			statusCode:        http.StatusInternalServerError,
			expectedRetryable: true,
			expectedBodyText:  strings.Repeat("x", 2048),
		},
		{
			name:              "input http 404 expects non-retryable failure",
			statusCode:        http.StatusNotFound,
			expectedRetryable: false,
			expectedBodyText:  "missing",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			webhookServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.WriteHeader(testCase.statusCode)
				_, _ = responseWriter.Write([]byte(testCase.expectedBodyText))
			}))
			defer webhookServer.Close()

			deliveryResult := NewHTTPClient(time.Second).Deliver(context.Background(), newDeliveryJob(webhookServer.URL))

			if deliveryResult.StatusCode != testCase.statusCode {
				t.Fatalf("expected status %d, got %#v", testCase.statusCode, deliveryResult)
			}
			if deliveryResult.ShouldRetry != testCase.expectedRetryable {
				t.Fatalf("expected retryable=%t, got %#v", testCase.expectedRetryable, deliveryResult)
			}
			expectedFailureReason := fmt.Sprintf("webhook returned status %d", testCase.statusCode)
			if deliveryResult.FailureReason != expectedFailureReason {
				t.Fatalf("expected failure reason %q, got %#v", expectedFailureReason, deliveryResult)
			}
			expectedResponseBodyLength := len(testCase.expectedBodyText)
			if expectedResponseBodyLength > 1024 {
				expectedResponseBodyLength = 1024
			}
			if len(deliveryResult.ResponseBody) != expectedResponseBodyLength {
				t.Fatalf("expected response body length %d, got %d", expectedResponseBodyLength, len(deliveryResult.ResponseBody))
			}
		})
	}
}

func TestIsRetryableTransportErrorRecognizesNetworkAndStringPatterns(t *testing.T) {
	testCases := []struct {
		name              string
		deliveryError     error
		expectedRetryable bool
	}{
		{
			name:              "input timeout net error expects retryable true",
			deliveryError:     timeoutNetError{message: "timed out"},
			expectedRetryable: true,
		},
		{
			name:              "input connection refused string expects retryable true",
			deliveryError:     errors.New("dial tcp: connection refused"),
			expectedRetryable: true,
		},
		{
			name:              "input generic error expects retryable false",
			deliveryError:     errors.New("bad certificate"),
			expectedRetryable: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			retryable := isRetryableTransportError(testCase.deliveryError)
			if retryable != testCase.expectedRetryable {
				t.Fatalf("expected retryable=%t, got %t", testCase.expectedRetryable, retryable)
			}
		})
	}
}

func TestIsRetryableStatusCodeRecognizesRetryableFamilies(t *testing.T) {
	// Input: retryable and non-retryable HTTP status codes.
	// Outcome: only 429 and 5xx responses are marked retryable.
	if !isRetryableStatusCode(http.StatusTooManyRequests) {
		t.Fatal("expected 429 to be retryable")
	}
	if !isRetryableStatusCode(http.StatusBadGateway) {
		t.Fatal("expected 502 to be retryable")
	}
	if isRetryableStatusCode(http.StatusBadRequest) {
		t.Fatal("expected 400 to be non-retryable")
	}
}

func newDeliveryJob(webhookURL string) events.DeliveryJob {
	return events.DeliveryJob{
		Event: events.SubscriberEvent{
			EventID:      "event-1",
			CustomerID:   "customer-a",
			SubscriberID: "subscriber-1",
			EventType:    "subscriber.created",
			OccurredAt:   time.Date(2026, time.August, 6, 10, 0, 0, 0, time.UTC),
		},
		WebhookURL: webhookURL,
		Attempt:    2,
		TraceID:    "trace-1",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type timeoutNetError struct {
	message string
}

func (deliveryError timeoutNetError) Error() string   { return deliveryError.message }
func (deliveryError timeoutNetError) Timeout() bool   { return true }
func (deliveryError timeoutNetError) Temporary() bool { return false }
func (deliveryError timeoutNetError) Unwrap() error   { return nil }

var _ net.Error = timeoutNetError{}
