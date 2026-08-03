package mockreceiver

import "webhook-notifier/internal/events"

func (application *Application) recordCustomerStatistic(customerID string, subscriberEvent *events.SubscriberEvent, decodeError error, statusCode int) {
	application.statisticsMutex.Lock()
	defer application.statisticsMutex.Unlock()

	customerStatistic, found := application.customerStats[customerID]
	if !found {
		customerStatistic = &CustomerStatistics{
			CustomerID:      customerID,
			EventTypeCounts: make(map[string]int64),
		}
		application.customerStats[customerID] = customerStatistic
	}

	customerStatistic.Received++
	if statusCode >= 200 && statusCode < 300 {
		customerStatistic.Success++
	} else {
		customerStatistic.Failed++
	}

	if decodeError != nil {
		customerStatistic.PayloadDecodeFailures++
		return
	}

	customerStatistic.LastEvent = subscriberEvent
	customerStatistic.EventTypeCounts[subscriberEvent.EventType]++
	if subscriberEvent.CustomerID != customerID {
		customerStatistic.PathPayloadCustomerMismatches++
	}
}

func (application *Application) snapshotCustomerStatistics() map[string]CustomerStatistics {
	application.statisticsMutex.RLock()
	defer application.statisticsMutex.RUnlock()

	customerStatistics := make(map[string]CustomerStatistics, len(application.customerStats))
	for customerID, customerStatistic := range application.customerStats {
		customerStatistics[customerID] = cloneCustomerStatistics(customerStatistic)
	}

	return customerStatistics
}

func (application *Application) snapshotCustomerStatistic(customerID string) CustomerStatistics {
	application.statisticsMutex.RLock()
	defer application.statisticsMutex.RUnlock()

	customerStatistic, found := application.customerStats[customerID]
	if !found {
		return CustomerStatistics{
			CustomerID:      customerID,
			EventTypeCounts: make(map[string]int64),
		}
	}

	return cloneCustomerStatistics(customerStatistic)
}

func cloneCustomerStatistics(customerStatistic *CustomerStatistics) CustomerStatistics {
	clonedStatistic := CustomerStatistics{
		CustomerID:                    customerStatistic.CustomerID,
		Received:                      customerStatistic.Received,
		Success:                       customerStatistic.Success,
		Failed:                        customerStatistic.Failed,
		PayloadDecodeFailures:         customerStatistic.PayloadDecodeFailures,
		PathPayloadCustomerMismatches: customerStatistic.PathPayloadCustomerMismatches,
		EventTypeCounts:               make(map[string]int64, len(customerStatistic.EventTypeCounts)),
	}

	if customerStatistic.LastEvent != nil {
		lastEvent := *customerStatistic.LastEvent
		clonedStatistic.LastEvent = &lastEvent
	}

	for eventType, count := range customerStatistic.EventTypeCounts {
		clonedStatistic.EventTypeCounts[eventType] = count
	}

	return clonedStatistic
}
