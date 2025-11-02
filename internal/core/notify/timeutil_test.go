package notify

import (
	"testing"
	"time"
)

func TestFormatShanghaiRFC3339(t *testing.T) {
	input := time.Date(2025, time.November, 2, 8, 0, 0, 0, time.UTC)
	got := formatShanghaiRFC3339(input)
	want := "2025-11-02T16:00:00+08:00"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
