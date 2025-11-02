package notify

import (
	"strings"
	"testing"
	"time"
)

func TestFormatLoginMessageIncludesLogPathAndShanghaiTime(t *testing.T) {
	event := LoginEvent{
		Type:      EventLoginSuccess,
		User:      "root",
		IP:        "1.2.3.4",
		Method:    "publickey",
		Port:      22,
		Timestamp: time.Date(2025, time.November, 2, 8, 30, 0, 0, time.UTC),
		Hostname:  "test-host",
		Location:  "Test Location",
		LogPath:   "/var/log/auth.log",
		Message:   "Accepted publickey",
	}

	content := formatLoginMessage(event)
	if !strings.Contains(content, "日志路径: /var/log/auth.log") {
		t.Fatalf("expected log path to appear in webhook message, got: %s", content)
	}

	expectedTime := "时间: 2025-11-02T16:30:00+08:00"
	if !strings.Contains(content, expectedTime) {
		t.Fatalf("expected timestamp %s, got: %s", expectedTime, content)
	}
}
