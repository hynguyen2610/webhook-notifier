package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"webhook-notifier/internal/events"
)

type HTTPClient struct {
	httpClient *http.Client
}

func NewHTTPClient(requestTimeout time.Duration) *HTTPClient {
	return &HTTPClient{
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

func (client *HTTPClient) Deliver(requestContext context.Context, job events.DeliveryJob) events.DeliveryResult {
	startedAt := time.Now()

	requestBody, marshalError := json.Marshal(job.Event)
	if marshalError != nil {
		return events.DeliveryResult{
			Job:           job,
			Duration:      time.Since(startedAt),
			FailureReason: fmt.Sprintf("marshal payload: %v", marshalError),
			CompletedAt:   time.Now(),
		}
	}

	request, requestError := http.NewRequestWithContext(requestContext, http.MethodPost, job.WebhookURL, bytes.NewReader(requestBody))
	if requestError != nil {
		return events.DeliveryResult{
			Job:           job,
			Duration:      time.Since(startedAt),
			FailureReason: fmt.Sprintf("create request: %v", requestError),
			CompletedAt:   time.Now(),
		}
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Event-ID", job.Event.EventID)
	request.Header.Set("X-Customer-ID", job.Event.CustomerID)
	request.Header.Set("X-Retry-Attempt", fmt.Sprintf("%d", job.Attempt))
	if job.TraceID != "" {
		request.Header.Set("X-Trace-ID", job.TraceID)
	}

	response, responseError := client.httpClient.Do(request)
	if responseError != nil {
		return events.DeliveryResult{
			Job:           job,
			Duration:      time.Since(startedAt),
			ShouldRetry:   isRetryableTransportError(responseError),
			FailureReason: responseError.Error(),
			CompletedAt:   time.Now(),
		}
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	result := events.DeliveryResult{
		Job:          job,
		StatusCode:   response.StatusCode,
		Duration:     time.Since(startedAt),
		ResponseBody: string(responseBody),
		CompletedAt:  time.Now(),
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return result
	}

	result.FailureReason = fmt.Sprintf("webhook returned status %d", response.StatusCode)
	result.ShouldRetry = isRetryableStatusCode(response.StatusCode)
	return result
}

func isRetryableTransportError(deliveryError error) bool {
	if netError, ok := deliveryError.(net.Error); ok {
		return netError.Timeout() || netError.Temporary()
	}

	lowerError := strings.ToLower(deliveryError.Error())
	return strings.Contains(lowerError, "timeout") || strings.Contains(lowerError, "connection reset") || strings.Contains(lowerError, "connection refused")
}

func isRetryableStatusCode(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || (statusCode >= 500 && statusCode <= 599)
}
