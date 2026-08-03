package mockgenerator

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func decodeJSONRequest(request *http.Request, destination any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	if decodeError := decoder.Decode(destination); decodeError != nil {
		return decodeError
	}

	var trailingPayload json.RawMessage
	if decodeError := decoder.Decode(&trailingPayload); decodeError != nil {
		if errors.Is(decodeError, io.EOF) {
			return nil
		}
		return errors.New("request body must contain a single JSON object")
	}

	return errors.New("request body must contain a single JSON object")
}

func validateGenerateRequest(generateRequest GenerateRequest) error {
	if strings.TrimSpace(generateRequest.CustomerID) == "" {
		return errors.New("customerId is required")
	}
	if generateRequest.Count < 0 {
		return errors.New("count must be zero or greater")
	}

	return nil
}

func validateBulkGenerateRequest(bulkRequest BulkGenerateRequest) error {
	if bulkRequest.Customers < 0 {
		return errors.New("customers must be zero or greater")
	}
	if bulkRequest.EventsPerCustomer < 0 {
		return errors.New("eventsPerCustomer must be zero or greater")
	}

	return nil
}
