package notify

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type EmailConfig struct {
	To     string
	From   string
	Server string
	User   string
	Pass   string
	Port   int
}

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
	location := event.Location
	if location == "" {
		location = "-"
	}
	method := event.Method
	if method == "" {
		method = "-"
	}
	port := "-"
	if event.Port > 0 {
		port = fmt.Sprintf("%d", event.Port)
	}
	message := event.Message
	if message == "" {
		message = "(无原始日志)"
	}

	logPath := strings.TrimSpace(event.LogPath)
	if logPath == "" {
		logPath = "-"
	}
	timestamp := formatShanghaiRFC3339(event.Timestamp)

	body := fmt.Sprintf(`
服务器登录提醒
-------------------
事件类型: %s
服务器: %s
用户: %s
来源IP: %s
来源端口: %s
认证方式: %s
位置: %s
时间: %s
日志路径: %s
日志: %s
`,
		event.Type,
		event.Hostname,
		event.User,
		event.IP,
		port,
		method,
		location,
		timestamp,
		logPath,
		message)

	msg := fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", e.To, e.From, subject, body)

	auth := smtp.PlainAuth("", e.Username, e.Password, e.Server)
	addr := fmt.Sprintf("%s:%d", e.Server, e.Port)

	// 旧实现直接使用 smtp.SendMail，无法控制超时且不支持 465 端口的隐式 TLS，会导致命令卡住。
	// return smtp.SendMail(addr, auth, e.From, []string{e.To}, []byte(msg))

	debugf("notify: 准备连接 SMTP %s", addr)
	if err := e.sendMailWithTimeout(addr, auth, []byte(msg)); err != nil {
		return err
	}
	debugf("notify: SMTP 发送完成")
	return nil
}

func (e *EmailNotifier) Test() error {
	testEvent := LoginEvent{
		Type:      "test",
		User:      "test_user",
		IP:        "127.0.0.1",
		Timestamp: time.Now(),
		Hostname:  "test_host",
		Location:  "Test Location",
		LogPath:   "-",
	}
	return e.Send(testEvent)
}

const (
	defaultDialTimeout  = 10 * time.Second
	defaultSMTPDeadline = 30 * time.Second
)

func needsImplicitTLS(port int) bool {
	return port == 465
}

func (e *EmailNotifier) sendMailWithTimeout(addr string, auth smtp.Auth, msg []byte) error {
	if err := validateSMTPLine(e.From); err != nil {
		return err
	}
	if err := validateSMTPLine(e.To); err != nil {
		return err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid smtp addr %s: %w", addr, err)
	}

	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect smtp server: %w", err)
	}

	deadline := time.Now().Add(defaultSMTPDeadline)
	_ = conn.SetDeadline(deadline)

	implicitTLS := needsImplicitTLS(e.Port)

	var client *smtp.Client
	if implicitTLS {
		debugf("notify: 使用隐式 TLS 连接 SMTP %s", addr)
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return fmt.Errorf("smtp tls handshake failed: %w", err)
		}
		_ = tlsConn.SetDeadline(deadline)
		client, err = smtp.NewClient(tlsConn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to create smtp client: %w", err)
	}
	defer client.Close()

	if !implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			debugf("notify: SMTP 支持 STARTTLS，将升级连接")
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("failed to start TLS: %w", err)
			}
		} else {
			debugf("notify: SMTP 不支持 STARTTLS，继续使用明文通道")
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp: server doesn't support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	if err := client.Mail(e.From); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}

	if err := client.Rcpt(e.To); err != nil {
		return fmt.Errorf("smtp rcpt to failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data begin failed: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp data write failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit failed: %w", err)
	}

	return nil
}

func validateSMTPLine(line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return fmt.Errorf("smtp: invalid line contains CR/LF")
	}
	return nil
}

func configureEmail(input EmailConfig) error {
	cfg := Config{
		Enabled:    true,
		Type:       "email",
		EmailTo:    input.To,
		EmailFrom:  input.From,
		SMTPServer: input.Server,
		SMTPPort:   input.Port,
		SMTPUser:   input.User,
		SMTPPass:   input.Pass,
	}

	if err := ValidateConfig(&cfg); err != nil {
		return err
	}

	notifier := NewEmailNotifier(cfg)
	if err := notifier.Test(); err != nil {
		return fmt.Errorf("email test failed: %v", err)
	}

	return saveConfig(cfg)
}
