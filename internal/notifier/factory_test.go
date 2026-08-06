package notifier

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/metrics"
	"webhook-notifier/internal/registration"
	"webhook-notifier/internal/scheduler"
	"webhook-notifier/internal/workqueue"
)

func swapApplicationFactories(t *testing.T) func() {
	t.Helper()
	originalRegistryStore := newRegistryStore
	originalQueueRepository := newQueueRepository
	originalScheduler := newScheduler
	originalDeliveryClientFactory := newDeliveryClientFactory
	originalNotifierMetrics := newNotifierMetrics

	return func() {
		newRegistryStore = originalRegistryStore
		newQueueRepository = originalQueueRepository
		newScheduler = originalScheduler
		newDeliveryClientFactory = originalDeliveryClientFactory
		newNotifierMetrics = originalNotifierMetrics
	}
}

func newPingMock(t *testing.T, _ error) (*sql.DB, sqlmock.Sqlmock, error) {
	t.Helper()
	return sqlmock.New(sqlmock.MonitorPingsOption(true))
}

func sqlExecMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, error) {
	t.Helper()
	return sqlmock.New()
}

var (
	_ = delivery.NewHTTPClient
	_ = metrics.NewNotifierMetrics
	_ = registration.NewPostgresRegistryWithDatabaseConnection
	_ = scheduler.NewRoundRobinScheduler
	_ = workqueue.NewPostgresRepositoryWithDatabaseConnection
)
