package registration

import "context"

type Registry interface {
	ResolveWebhookURLs(requestContext context.Context, customerID string) ([]string, error)
	Snapshot(requestContext context.Context) (map[string][]string, error)
	Close() error
}
