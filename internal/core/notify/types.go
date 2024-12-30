package notify

import "time"

// NotifyType 定义通知类型
type NotifyType int

const (
	NotifyTypeWebhook NotifyType = iota
	NotifyTypeEmail
)

// LoginEvent 定义登录事件
type LoginEvent struct {
	Type      string    // 事件类型：login_success 或 login_failed
	User      string    // 登录用户
	IP        string    // 来源IP
	Timestamp time.Time // 事件时间
	Hostname  string    // 主机名
	Location  string    // IP地理位置（可选）
}

// Notifier 定义通知接口
type Notifier interface {
	// Send 发送通知
	Send(event LoginEvent) error
	// Test 测试通知配置
	Test() error
}

// Config 通知配置
type Config struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	Type        string `json:"type" yaml:"type"`
	WebhookURL  string `json:"webhook_url,omitempty" yaml:"webhook_url,omitempty"`
	EmailTo     string `json:"email_to,omitempty" yaml:"email_to,omitempty"`
	EmailFrom   string `json:"email_from,omitempty" yaml:"email_from,omitempty"`
	SMTPServer  string `json:"smtp_server,omitempty" yaml:"smtp_server,omitempty"`
	SMTPPort    int    `json:"smtp_port,omitempty" yaml:"smtp_port,omitempty"`
	SMTPUser    string `json:"smtp_user,omitempty" yaml:"smtp_user,omitempty"`
	SMTPPass    string `json:"smtp_pass,omitempty" yaml:"smtp_pass,omitempty"`
}
