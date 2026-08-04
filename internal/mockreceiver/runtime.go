package mockreceiver

import (
	"context"
	"errors"
	"net/http"
)

func (application *Application) Run(requestContext context.Context) error {
	serverErrors := make(chan error, 1)
	go func() {
		application.logger.Info("starting mock receiver", "address", application.config.HTTPAddress)
		listenError := application.httpServer.ListenAndServe()
		if listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
			serverErrors <- listenError
			return
		}
		serverErrors <- nil
	}()

	select {
	case <-requestContext.Done():
	case serverError := <-serverErrors:
		if serverError != nil {
			return serverError
		}
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), application.config.ShutdownTimeout)
	defer cancelShutdown()

	return application.httpServer.Shutdown(shutdownContext)
}
