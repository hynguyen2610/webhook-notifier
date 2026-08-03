package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"webhook-notifier/internal/delivery"
	"webhook-notifier/internal/scheduler"
)

type workerScalingSummary struct {
	workerCount   int
	jobCount      int
	totalDuration time.Duration
	jobsPerSecond float64
	speedup       float64
	efficiency    float64
}

func runWorkerScalingLoadTest() ([]workerScalingSummary, error) {
	scenario := newScenario("worker-scaling", map[string]int{
		"customer-a": 60,
		"customer-b": 60,
		"customer-c": 60,
		"customer-d": 60,
	})
	receiverDelay := 15 * time.Millisecond
	workerCounts := []int{1, 2, 4, 8}

	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		time.Sleep(receiverDelay)
		_, _ = io.Copy(io.Discard, request.Body)
		_ = request.Body.Close()
		responseWriter.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	baselineDuration := time.Duration(0)
	summaries := make([]workerScalingSummary, 0, len(workerCounts))
	for _, workerCount := range workerCounts {
		summary, loadTestError := runWorkerScalingScenario(scenario, server.URL, workerCount)
		if loadTestError != nil {
			return nil, loadTestError
		}
		if baselineDuration == 0 {
			baselineDuration = summary.totalDuration
			summary.speedup = 1
			summary.efficiency = 1
		} else {
			summary.speedup = float64(baselineDuration) / float64(summary.totalDuration)
			summary.efficiency = summary.speedup / float64(workerCount)
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func runWorkerScalingScenario(scenario benchmarkScenario, serverURL string, workerCount int) (workerScalingSummary, error) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	roundRobinScheduler := scheduler.NewRoundRobinScheduler(len(scenario.jobs))
	scheduledJobs := roundRobinScheduler.Start(requestContext)
	deliveryClient := delivery.NewHTTPClient(3 * time.Second)

	startedAt := time.Now()

	var workerGroup sync.WaitGroup
	var deliveryGroup sync.WaitGroup
	deliveryGroup.Add(len(scenario.jobs))
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for {
				select {
				case <-requestContext.Done():
					return
				case job, channelOpen := <-scheduledJobs:
					if !channelOpen {
						return
					}
					job.WebhookURL = serverURL
					_ = deliveryClient.Deliver(requestContext, job)
					deliveryGroup.Done()
				}
			}
		}()
	}

	for _, job := range scenario.jobs {
		job.WebhookURL = serverURL
		roundRobinScheduler.Enqueue(job)
	}

	deliveryGroup.Wait()
	roundRobinScheduler.Close()
	cancelRequest()
	workerGroup.Wait()

	totalDuration := time.Since(startedAt)
	if totalDuration <= 0 {
		totalDuration = time.Nanosecond
	}

	return workerScalingSummary{
		workerCount:   workerCount,
		jobCount:      len(scenario.jobs),
		totalDuration: totalDuration,
		jobsPerSecond: float64(len(scenario.jobs)) / totalDuration.Seconds(),
	}, nil
}
