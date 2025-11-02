package notify

import (
	"testing"
	"time"
)

func TestShouldSkipHistoricalEvent(t *testing.T) {
	start := mustParseTime(t, "2025-01-01T10:00:00Z")
	caseNew := start.Add(-journalHistoryTolerance / 2)
	caseOld := start.Add(-journalHistoryTolerance * 2)
	caseFuture := start.Add(time.Second)

	if shouldSkipHistoricalEvent(start, caseNew) {
		t.Fatalf("expected not to skip event within tolerance")
	}

	if !shouldSkipHistoricalEvent(start, caseOld) {
		t.Fatalf("expected to skip old event beyond tolerance")
	}

	if shouldSkipHistoricalEvent(start, caseFuture) {
		t.Fatalf("should not skip future event")
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time failed: %v", err)
	}
	return parsed
}
