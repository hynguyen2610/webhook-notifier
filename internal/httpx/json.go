package httpx

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(responseWriter http.ResponseWriter, statusCode int, payload any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	if payload == nil {
		return
	}

	_ = json.NewEncoder(responseWriter).Encode(payload)
}

func WriteError(responseWriter http.ResponseWriter, statusCode int, message string) {
	WriteJSON(responseWriter, statusCode, map[string]string{
		"error": message,
	})
}
