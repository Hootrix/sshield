package notify

import "time"

// NotifyType 定义通知类型
type NotifyType int

const (
	NotifyTypeWebhook NotifyType = iota
	NotifyTypeEmail
)

const (
	EventLoginSuccess = "login_success"
	EventLoginFailed  = "login_failed"
)

// LoginEvent 定义登录事件
type LoginEvent struct {
	Type      string    // 事件类型：login_success 或 login_failed
	User      string    // 登录用户
	IP        string    // 来源IP
	Method    string    // 认证方式 password/publickey/keyboard-interactive
	Port      int       // 来源端口
	Timestamp time.Time // 事件时间
	Hostname  string    // 主机名
	Location  string    // IP地理位置（可选）
	LogPath   string    // 日志来源路径（文件路径或 journald 单元）
	Message   string    // 原始日志消息
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
	Channels []ChannelConfig `json:"channels" yaml:"channels"`
}

// ChannelConfig 单个通知渠道配置
type ChannelConfig struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"` // 渠道名称（可选，用于显示）
	Enabled bool   `json:"enabled" yaml:"enabled"`               // 是否启用
	Type    string `json:"type" yaml:"type"`                     // 类型：curl/email

	// 不同类型的配置，根据 Type 使用对应字段
	Curl  *CurlConfig  `json:"curl,omitempty" yaml:"curl,omitempty"`
	Email *EmailConfig `json:"email,omitempty" yaml:"email,omitempty"`
}

// CurlConfig 自定义 Curl 通知配置
// 支持模板变量：{{.Type}} {{.User}} {{.IP}} {{.Port}} {{.Method}} {{.Hostname}} {{.Timestamp}} {{.Location}} {{.LogPath}} {{.Message}}
type CurlConfig struct {
	Command string `json:"command" yaml:"command"`
}

// EmailConfig 邮件通知配置
type EmailConfig struct {
	To     string `json:"to" yaml:"to"`
	From   string `json:"from" yaml:"from"`
	Server string `json:"server" yaml:"server"`
	Port   int    `json:"port" yaml:"port"`
	User   string `json:"user" yaml:"user"`
	Pass   string `json:"pass" yaml:"pass"`
}

// GetEnabledChannels 获取所有启用的渠道配置
func (c *Config) GetEnabledChannels() []ChannelConfig {
	var channels []ChannelConfig
	for _, ch := range c.Channels {
		if ch.Enabled {
			channels = append(channels, ch)
		}
	}
	return channels
}
