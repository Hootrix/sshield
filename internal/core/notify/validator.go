package notify

import (
	"net/mail"
	"net/url"
	"strings"
)

// ConfigValidator 配置验证器接口
type ConfigValidator interface {
	Validate() error
}

// ValidateConfig 验证通知配置
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrConfigInvalid
	}

	validationErr := &ValidationError{}

	// 验证基本配置
	if !cfg.Enabled {
		return ErrNotEnabled
	}

	// 验证通知类型
	switch strings.ToLower(cfg.Type) {
	case "webhook":
		validateWebhookConfig(cfg, validationErr)
	case "email":
		validateEmailConfig(cfg, validationErr)
	default:
		validationErr.AddError("type", "unsupported notification type")
	}

	if validationErr.HasErrors() {
		return validationErr
	}

	return nil
}

// validateWebhookConfig 验证 Webhook 配置
func validateWebhookConfig(cfg *Config, validationErr *ValidationError) {
	if cfg.WebhookURL == "" {
		validationErr.AddError("webhook_url", "webhook URL is required")
		return
	}

	// 验证 URL 格式
	_, err := url.ParseRequestURI(cfg.WebhookURL)
	if err != nil {
		validationErr.AddError("webhook_url", "invalid webhook URL format")
	}

	// 验证 URL 协议
	if !strings.HasPrefix(cfg.WebhookURL, "https://") && !strings.HasPrefix(cfg.WebhookURL, "http://") {
		validationErr.AddError("webhook_url", "webhook URL must start with http:// or https://")
	}
}

// validateEmailConfig 验证邮件配置
func validateEmailConfig(cfg *Config, validationErr *ValidationError) {
	// 验证收件人邮箱
	if cfg.EmailTo == "" {
		validationErr.AddError("email_to", "recipient email is required")
	} else if _, err := mail.ParseAddress(cfg.EmailTo); err != nil {
		validationErr.AddError("email_to", "invalid recipient email format")
	}

	// 验证发件人邮箱
	if cfg.EmailFrom == "" {
		validationErr.AddError("email_from", "sender email is required")
	} else if _, err := mail.ParseAddress(cfg.EmailFrom); err != nil {
		validationErr.AddError("email_from", "invalid sender email format")
	}

	// 验证 SMTP 服务器配置
	if cfg.SMTPServer == "" {
		validationErr.AddError("smtp_server", "SMTP server is required")
	}

	if cfg.SMTPPort <= 0 || cfg.SMTPPort > 65535 {
		validationErr.AddError("smtp_port", "invalid SMTP port")
	}

	// 验证认证信息
	if cfg.SMTPUser == "" {
		validationErr.AddError("smtp_user", "SMTP username is required")
	}

	if cfg.SMTPPass == "" {
		validationErr.AddError("smtp_pass", "SMTP password is required")
	}
}
