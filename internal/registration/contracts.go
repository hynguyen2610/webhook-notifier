package registration

import (
	"context"
	"errors"
)

var ErrCustomerNotRegistered = errors.New("customer not registered")

type Registry interface {
	ResolveWebhookURLs(requestContext context.Context, customerID string) ([]string, error)
	Snapshot(requestContext context.Context) (map[string][]string, error)
	Close() error
}
