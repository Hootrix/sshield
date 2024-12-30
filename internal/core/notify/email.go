package notify

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type EmailNotifier struct {
	To       string
	From     string
	Server   string
	Port     int
	Username string
	Password string
}

func NewEmailNotifier(config Config) *EmailNotifier {
	return &EmailNotifier{
		To:       config.EmailTo,
		From:     config.EmailFrom,
		Server:   config.SMTPServer,
		Port:     config.SMTPPort,
		Username: config.SMTPUser,
		Password: config.SMTPPass,
	}
}

func (e *EmailNotifier) Send(event LoginEvent) error {
	subject := fmt.Sprintf("服务器登录提醒 - %s", event.Type)
	body := fmt.Sprintf(`
服务器登录提醒
-------------------
事件类型: %s
服务器: %s
用户: %s
来源IP: %s
位置: %s
时间: %s
`,
		event.Type,
		event.Hostname,
		event.User,
		event.IP,
		event.Location,
		event.Timestamp.Format(time.RFC3339))

	msg := fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", e.To, e.From, subject, body)

	auth := smtp.PlainAuth("", e.Username, e.Password, e.Server)
	addr := fmt.Sprintf("%s:%d", e.Server, e.Port)

	return smtp.SendMail(addr, auth, e.From, []string{e.To}, []byte(msg))
}

func (e *EmailNotifier) Test() error {
	testEvent := LoginEvent{
		Type:      "test",
		User:      "test_user",
		IP:        "127.0.0.1",
		Timestamp: time.Now(),
		Hostname:  "test_host",
		Location:  "Test Location",
	}
	return e.Send(testEvent)
}

func configureEmail(email string) error {
	// 这里需要用户提供SMTP配置
	cfg := Config{
		Enabled:  true,
		Type:     "email",
		EmailTo:  email,
		// 其他SMTP配置需要从配置文件或环境变量读取
	}

	notifier := NewEmailNotifier(cfg)
	if err := notifier.Test(); err != nil {
		return fmt.Errorf("email test failed: %v", err)
	}

	return saveConfig(cfg)
}
