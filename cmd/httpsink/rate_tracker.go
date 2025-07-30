package main

import (
	"log"
	"sync"
	"time"
)

// SpanRateTracker tracks spans received per second
type SpanRateTracker struct {
	mu             sync.RWMutex
	spanCounts     map[int64]int // Map of timestamp (seconds) to span count
	startTime      time.Time     // When tracking started
	totalSpans     int           // Total spans counted by the tracker
	lastReportTime time.Time     // Last time stats were reported
	reportInterval time.Duration // How often to report stats
}

// NewSpanRateTracker creates a new rate tracker
func NewSpanRateTracker() *SpanRateTracker {
	return &SpanRateTracker{
		spanCounts:     make(map[int64]int),
		startTime:      time.Now(),
		lastReportTime: time.Now(),
		reportInterval: 5 * time.Second, // Report every 10 seconds
	}
}

// TrackSpans adds span count to the current second
func (t *SpanRateTracker) TrackSpans(count int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Use Unix timestamp as the key (second precision)
	now := time.Now()
	key := now.Unix()

	t.spanCounts[key] += count
	t.totalSpans += count

	// Report periodically
	if now.Sub(t.lastReportTime) >= t.reportInterval {
		t.reportStats()
		t.lastReportTime = now
	}
}

// GetCurrentRate returns the average spans/second over the last n seconds
func (t *SpanRateTracker) GetCurrentRate(seconds int) float64 {

	now := time.Now()
	cutoff := now.Add(-time.Duration(seconds) * time.Second).Unix()

	var total int
	for ts, count := range t.spanCounts {
		if ts >= cutoff {
			total += count
		}
	}

	// If we have less than n seconds of data, use what we have
	actualSeconds := int64(seconds)
	elapsedSeconds := now.Unix() - t.startTime.Unix()
	if elapsedSeconds < int64(seconds) {
		actualSeconds = elapsedSeconds
		if actualSeconds == 0 {
			actualSeconds = 1 // Avoid division by zero
		}
	}

	return float64(total) / float64(actualSeconds)
}

// reportStats logs the current rate statistics
func (t *SpanRateTracker) reportStats() {
	// Get rates for different time windows
	rate1s := t.GetCurrentRate(1)
	rate10s := t.GetCurrentRate(10)
	rate60s := t.GetCurrentRate(60)

	log.Printf("Spans per second: %.2f (1s) | %.2f (10s) | %.2f (60s) | Total: %d",
		rate1s, rate10s, rate60s, t.totalSpans)

}

// GetRateSummary returns a summary of the rate statistics
func (t *SpanRateTracker) GetRateSummary() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Calculate rates for different time windows
	now := time.Now()
	runningTime := now.Sub(t.startTime).Seconds()

	return map[string]interface{}{
		"spans_per_second_1s":  t.GetCurrentRate(1),
		"spans_per_second_10s": t.GetCurrentRate(10),
		"spans_per_second_60s": t.GetCurrentRate(60),
		"total_spans":          t.totalSpans,
		"running_time_seconds": runningTime,
		"average_rate":         float64(t.totalSpans) / runningTime,
	}
}
