package notify

import (
	"fmt"
	"net/mail"
	"strings"
)

// ConfigValidator 配置验证器接口
type ConfigValidator interface {
	Validate() error
}

// ValidateConfig 验证整个配置
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrConfigInvalid
	}

	for i, ch := range cfg.Channels {
		if err := ValidateChannelConfig(&ch); err != nil {
			return fmt.Errorf("渠道 %d (%s): %w", i+1, ch.Type, err)
		}
	}

	return nil
}

// ValidateChannelConfig 验证渠道配置
func ValidateChannelConfig(ch *ChannelConfig) error {
	if ch == nil {
		return ErrConfigInvalid
	}

	validationErr := &ValidationError{}

	switch strings.ToLower(ch.Type) {
	case "curl":
		validateCurlChannel(ch, validationErr)
	case "email":
		validateEmailChannel(ch, validationErr)
	default:
		validationErr.AddError("type", "unsupported notification type: "+ch.Type)
	}

	if validationErr.HasErrors() {
		return validationErr
	}

	return nil
}

// validateCurlChannel 验证 Curl 渠道配置
func validateCurlChannel(ch *ChannelConfig, validationErr *ValidationError) {
	if ch.Curl == nil {
		validationErr.AddError("curl", "curl config is required")
		return
	}

	if ch.Curl.Command == "" {
		validationErr.AddError("curl.command", "curl command is required")
	}
}

// validateEmailChannel 验证邮件渠道配置
func validateEmailChannel(ch *ChannelConfig, validationErr *ValidationError) {
	if ch.Email == nil {
		validationErr.AddError("email", "email config is required")
		return
	}

	e := ch.Email

	// 验证收件人邮箱
	if e.To == "" {
		validationErr.AddError("email.to", "recipient email is required")
	} else if _, err := mail.ParseAddress(e.To); err != nil {
		validationErr.AddError("email.to", "invalid recipient email format")
	}

	// 验证发件人邮箱
	if e.From == "" {
		validationErr.AddError("email.from", "sender email is required")
	} else if _, err := mail.ParseAddress(e.From); err != nil {
		validationErr.AddError("email.from", "invalid sender email format")
	}

	// 验证 SMTP 服务器配置
	if e.Server == "" {
		validationErr.AddError("email.server", "SMTP server is required")
	}

	if e.Port <= 0 || e.Port > 65535 {
		validationErr.AddError("email.port", "invalid SMTP port")
	}

	// 验证认证信息
	if e.User == "" {
		validationErr.AddError("email.user", "SMTP username is required")
	}

	if e.Pass == "" {
		validationErr.AddError("email.pass", "SMTP password is required")
	}
}
