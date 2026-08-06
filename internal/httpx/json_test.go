package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONWritesHeadersAndPayload(t *testing.T) {
	// Input: a JSON response with a success payload.
	// Outcome: the helper writes the status code, content type, and encoded JSON body.
	responseRecorder := httptest.NewRecorder()

	WriteJSON(responseRecorder, http.StatusAccepted, map[string]string{"status": "queued"})

	if responseRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", responseRecorder.Code)
	}
	if responseRecorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected JSON content type, got %s", responseRecorder.Header().Get("Content-Type"))
	}

	var payload map[string]string
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode JSON payload: %v", decodeError)
	}
	if payload["status"] != "queued" {
		t.Fatalf("expected status payload, got %#v", payload)
	}
}

func TestWriteJSONAllowsNilPayload(t *testing.T) {
	// Input: a JSON response with a nil payload.
	// Outcome: the helper writes only headers and status without a body.
	responseRecorder := httptest.NewRecorder()

	WriteJSON(responseRecorder, http.StatusNoContent, nil)

	if responseRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", responseRecorder.Code)
	}
	if responseRecorder.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", responseRecorder.Body.String())
	}
}

func TestWriteErrorWrapsMessageInErrorPayload(t *testing.T) {
	// Input: an error message and HTTP 400 status.
	// Outcome: the helper writes a JSON object with the error field populated.
	responseRecorder := httptest.NewRecorder()

	WriteError(responseRecorder, http.StatusBadRequest, "customerId is required")

	var payload map[string]string
	if decodeError := json.Unmarshal(responseRecorder.Body.Bytes(), &payload); decodeError != nil {
		t.Fatalf("decode JSON error payload: %v", decodeError)
	}
	if payload["error"] != "customerId is required" {
		t.Fatalf("expected error payload, got %#v", payload)
	}
}

