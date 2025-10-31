package notify

import (
	"errors"
	"fmt"
	"os"
	"time"
)

func testNotification() error {
	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("notification is not configured")
		}
		return fmt.Errorf("load config failed: %w", err)
	}

	if cfg == nil || !cfg.Enabled {
		return fmt.Errorf("notification is not enabled")
	}

	notifier, err := buildNotifier(*cfg)
	if err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	event := LoginEvent{
		Type:      "test",
		User:      "sshield",
		IP:        "127.0.0.1",
		Method:    "manual",
		Port:      0,
		Timestamp: time.Now(),
		Hostname:  hostname,
		Message:   "test notification",
	}

	return notifier.Send(event)
}
