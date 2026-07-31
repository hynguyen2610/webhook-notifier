package registration

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var ErrCustomerNotRegistered = errors.New("customer not registered")

type MemoryRegistry struct {
	mutex            sync.RWMutex
	customerWebhooks map[string][]string
}

func NewMemoryRegistry(initialRegistrations map[string][]string) *MemoryRegistry {
	copiedRegistrations := make(map[string][]string, len(initialRegistrations))
	for customerID, webhookURLs := range initialRegistrations {
		copiedWebhookURLs := append([]string(nil), webhookURLs...)
		copiedRegistrations[customerID] = copiedWebhookURLs
	}

	return &MemoryRegistry{
		customerWebhooks: copiedRegistrations,
	}
}

func (registry *MemoryRegistry) ResolveWebhookURLs(_ context.Context, customerID string) ([]string, error) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	webhookURLs, found := registry.customerWebhooks[customerID]
	if !found || len(webhookURLs) == 0 {
		return nil, ErrCustomerNotRegistered
	}

	return append([]string(nil), webhookURLs...), nil
}

func (registry *MemoryRegistry) Snapshot(_ context.Context) (map[string][]string, error) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	snapshot := make(map[string][]string, len(registry.customerWebhooks))
	for customerID, webhookURLs := range registry.customerWebhooks {
		snapshot[customerID] = append([]string(nil), webhookURLs...)
	}

	return snapshot, nil
}

func (registry *MemoryRegistry) SortedCustomers() []string {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	customerIDs := make([]string, 0, len(registry.customerWebhooks))
	for customerID := range registry.customerWebhooks {
		customerIDs = append(customerIDs, customerID)
	}
	sort.Strings(customerIDs)
	return customerIDs
}

func (registry *MemoryRegistry) Close() error {
	return nil
}
