package main

import (
	"sort"

	"webhook-notifier/internal/events"
)

var syntheticDeliverySink int

func sortedCustomerIDs(jobs []events.DeliveryJob) []string {
	customerSet := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		customerSet[job.Event.CustomerID] = struct{}{}
	}

	customerIDs := make([]string, 0, len(customerSet))
	for customerID := range customerSet {
		customerIDs = append(customerIDs, customerID)
	}
	sort.Strings(customerIDs)
	return customerIDs
}

func customerJobCounts(jobs []events.DeliveryJob) map[string]int {
	counts := make(map[string]int)
	for _, job := range jobs {
		counts[job.Event.CustomerID]++
	}
	return counts
}

func runSyntheticDelivery(job events.DeliveryJob, syntheticWorkIterations int) {
	payload := job.Event.EventID + job.Event.CustomerID + job.Event.SubscriberID
	if payload == "" {
		payload = "event"
	}

	checksum := 0
	for iterationIndex := 0; iterationIndex < syntheticWorkIterations; iterationIndex++ {
		payloadIndex := iterationIndex % len(payload)
		checksum += int(payload[payloadIndex]) + iterationIndex
	}

	syntheticDeliverySink += checksum
}
