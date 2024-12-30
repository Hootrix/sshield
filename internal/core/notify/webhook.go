package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WebhookNotifier struct {
	URL        string
	httpClient *http.Client
}

type webhookMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// NewWebhookNotifier creates a new webhook notifier with timeout
func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{
		URL: url,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send sends a notification via webhook
func (w *WebhookNotifier) Send(event LoginEvent) error {
	if w.URL == "" {
		return &ConfigError{
			Field:   "webhook_url",
			Message: "webhook URL is empty",
		}
	}

	msg := webhookMessage{
		MsgType: "text",
		Text: struct {
			Content string `json:"content"`
		}{
			Content: formatLoginMessage(event),
		},
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook message: %w", err)
	}

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", w.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Test tests the webhook configuration
func (w *WebhookNotifier) Test() error {
	testEvent := LoginEvent{
		Type:      "test",
		User:      "test_user",
		IP:        "127.0.0.1",
		Timestamp: time.Now(),
		Hostname:  "test_host",
		Location:  "Test Location",
	}

	if err := w.Send(testEvent); err != nil {
		return fmt.Errorf("webhook test failed: %w", err)
	}

	return nil
}

// formatLoginMessage formats the login event message
func formatLoginMessage(event LoginEvent) string {
	return fmt.Sprintf(`服务器登录提醒
事件类型: %s
服务器: %s
用户: %s
来源IP: %s
位置: %s
时间: %s`,
		event.Type,
		event.Hostname,
		event.User,
		event.IP,
		event.Location,
		event.Timestamp.Format(time.RFC3339))
}

// configureWebhook configures and tests the webhook notification
func configureWebhook(url string) error {
	// 创建配置管理器
	cm := NewConfigManager()

	// 创建新的配置
	cfg := Config{
		Enabled:    true,
		Type:       "webhook",
		WebhookURL: url,
	}

	// 验证配置
	if err := ValidateConfig(&cfg); err != nil {
		return err
	}

	// 测试 webhook
	notifier := NewWebhookNotifier(url)
	if err := notifier.Test(); err != nil {
		return err
	}

	// 备份当前配置
	if cm.configExists() {
		if err := cm.BackupConfig(); err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
	}

	// 保存新配置
	if err := cm.SaveConfig(cfg); err != nil {
		// 如果保存失败，尝试恢复备份
		if restoreErr := cm.RestoreConfig(); restoreErr != nil {
			return fmt.Errorf("failed to save config and restore backup: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}
