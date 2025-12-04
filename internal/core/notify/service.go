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
			return fmt.Errorf("未配置通知渠道")
		}
		return fmt.Errorf("加载配置失败: %w", err)
	}

	channels := cfg.GetEnabledChannels()
	if len(channels) == 0 {
		return fmt.Errorf("没有启用的通知渠道")
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
		LogPath:   "-",
	}

	var errs []error
	for _, ch := range channels {
		notifier, err := buildChannelNotifier(ch)
		if err != nil {
			errs = append(errs, fmt.Errorf("渠道 %s: %w", ch.Type, err))
			continue
		}
		fmt.Printf("正在测试渠道: %s...\n", ch.Type)
		if err := notifier.Send(event); err != nil {
			errs = append(errs, fmt.Errorf("渠道 %s: %w", ch.Type, err))
		} else {
			fmt.Printf("✓ 渠道 %s 测试成功\n", ch.Type)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分渠道测试失败: %v", errs)
	}
	return nil
}
