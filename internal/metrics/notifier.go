package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type NotifierMetrics struct {
	ReceivedEventsCounter     prometheus.Counter
	DeliveredEventsCounter    *prometheus.CounterVec
	FailedDeliveriesCounter   *prometheus.CounterVec
	RetriedDeliveriesCounter  prometheus.Counter
	DeadLetterCounter         prometheus.Counter
	DeliveryDurationHistogram *prometheus.HistogramVec
	ScheduledQueueDepthGauge  prometheus.Gauge
	PendingQueueDepthGauge    prometheus.Gauge
	OldestPendingAgeGauge     prometheus.Gauge
}

func NewNotifierMetrics() *NotifierMetrics {
	return &NotifierMetrics{
		ReceivedEventsCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "webhook_notifier_received_events_total",
			Help: "Total number of subscriber events accepted by the notifier.",
		}),
		DeliveredEventsCounter: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "webhook_notifier_delivered_events_total",
			Help: "Total number of successful webhook deliveries.",
		}, []string{"customer_id"}),
		FailedDeliveriesCounter: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "webhook_notifier_failed_deliveries_total",
			Help: "Total number of failed webhook deliveries.",
		}, []string{"customer_id", "retryable"}),
		RetriedDeliveriesCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "webhook_notifier_retried_deliveries_total",
			Help: "Total number of webhook deliveries scheduled for retry.",
		}),
		DeadLetterCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "webhook_notifier_dead_letter_total",
			Help: "Total number of webhook deliveries moved to dead letter handling.",
		}),
		DeliveryDurationHistogram: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "webhook_notifier_delivery_duration_seconds",
			Help:    "Webhook delivery duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"customer_id", "status_family"}),
		ScheduledQueueDepthGauge: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "webhook_notifier_scheduled_queue_depth",
			Help: "Approximate number of queued delivery jobs waiting for workers.",
		}),
		PendingQueueDepthGauge: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "webhook_notifier_pending_queue_depth",
			Help: "Number of pending delivery rows currently waiting in PostgreSQL.",
		}),
		OldestPendingAgeGauge: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "webhook_notifier_oldest_pending_event_age_seconds",
			Help: "Age in seconds of the oldest pending delivery row in PostgreSQL.",
		}),
	}
}

func Handler() http.Handler {
	return promhttp.Handler()
}
