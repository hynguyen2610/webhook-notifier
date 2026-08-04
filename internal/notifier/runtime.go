package notifier

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"webhook-notifier/internal/events"
)

func (application *Application) Run(requestContext context.Context) error {
	scheduledJobs := application.scheduler.Start(requestContext)
	application.startMetricsReporter(requestContext)

	var workerGroup sync.WaitGroup
	application.startWorkers(requestContext, scheduledJobs, &workerGroup)

	serverErrors := make(chan error, 1)
	queueErrors := make(chan error, 1)

	go application.serveHTTP(serverErrors)
	go application.pollQueue(requestContext, queueErrors)

	if runError := application.waitForRuntimeCompletion(requestContext, serverErrors, queueErrors); runError != nil {
		application.scheduler.Close()
		return runError
	}

	application.scheduler.Close()

	return application.shutdown(workerGroup)
}

func (application *Application) startWorkers(requestContext context.Context, scheduledJobs <-chan events.DeliveryJob, workerGroup *sync.WaitGroup) {
	for workerIndex := 0; workerIndex < application.config.WorkerCount; workerIndex++ {
		workerGroup.Add(1)
		go func(workerID int) {
			defer workerGroup.Done()
			application.runWorker(requestContext, workerID, scheduledJobs)
		}(workerIndex + 1)
	}
}

func (application *Application) serveHTTP(serverErrors chan<- error) {
	application.logger.Info("starting notifier", "address", application.config.HTTPAddress, "workerCount", application.config.WorkerCount)
	listenError := application.httpServer.ListenAndServe()
	if listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
		serverErrors <- listenError
		return
	}
	serverErrors <- nil
}

func (application *Application) pollQueue(requestContext context.Context, queueErrors chan<- error) {
	queueErrors <- application.runQueuePoller(requestContext)
}

func (application *Application) waitForRuntimeCompletion(requestContext context.Context, serverErrors <-chan error, queueErrors <-chan error) error {
	select {
	case <-requestContext.Done():
		return nil
	case serverError := <-serverErrors:
		return serverError
	case queueError := <-queueErrors:
		return queueError
	}
}

func (application *Application) shutdown(workerGroup sync.WaitGroup) error {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), application.config.ShutdownTimeout)
	defer cancelShutdown()

	shutdownError := application.httpServer.Shutdown(shutdownContext)
	registryCloseError := application.registry.Close()
	queueCloseError := application.workQueue.Close()
	workerGroup.Wait()
	if shutdownError != nil {
		return shutdownError
	}
	if registryCloseError != nil {
		return registryCloseError
	}
	if queueCloseError != nil {
		return queueCloseError
	}

	return nil
}
