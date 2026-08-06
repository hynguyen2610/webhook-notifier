package notifier

import (
	"net/http"

	"webhook-notifier/internal/metrics"
)

func (application *Application) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:    application.config.HTTPAddress,
		Handler: application.newRouter(),
	}
}

func (application *Application) newRouter() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", application.handleHealth)
	router.Handle("GET /metrics", metrics.Handler())
	router.HandleFunc("GET /stats", application.handleStats)
	router.HandleFunc("GET /registrations", application.handleRegistrations)
	router.HandleFunc("GET /dlq", application.handleDeadLetters)
	router.HandleFunc("POST /events", application.handleSingleEvent)
	router.HandleFunc("POST /events/batch", application.handleBatchEvents)
	return router
}
