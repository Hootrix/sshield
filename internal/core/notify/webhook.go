package notify

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CurlNotifier 使用自定义 curl 命令的通知器
type CurlNotifier struct {
	curlCmd    string
	parsedCurl *CurlRequest
	httpClient *http.Client
}

// NewCurlNotifier 创建基于 curl 命令的通知器
func NewCurlNotifier(curlCmd string) (*CurlNotifier, error) {
	parsed, err := ParseCurl(curlCmd)
	if err != nil {
		return nil, fmt.Errorf("解析 curl 命令失败: %w", err)
	}

	return &CurlNotifier{
		curlCmd:    curlCmd,
		parsedCurl: parsed,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Send 使用 curl 命令发送通知
func (c *CurlNotifier) Send(event LoginEvent) error {
	// 构建模板数据
	data := map[string]any{
		"Type":      event.Type,
		"User":      event.User,
		"IP":        event.IP,
		"Port":      event.Port,
		"Method":    event.Method,
		"Hostname":  event.Hostname,
		"Timestamp": formatShanghaiRFC3339(event.Timestamp),
		"Location":  event.Location,
		"LogPath":   event.LogPath,
		"Message":   event.Message,
		"HostIP":    event.HostIP,
	}

	resp, err := c.parsedCurl.Execute(data)
	if err != nil {
		return fmt.Errorf("执行 curl 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("请求失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Test 测试 curl 配置
func (c *CurlNotifier) Test() error {
	testEvent := LoginEvent{
		Type:      "test",
		User:      "test_user",
		IP:        "127.0.0.1",
		Timestamp: time.Now(),
		Hostname:  "test_host",
		Location:  "Test Location",
		LogPath:   "-",
	}

	if err := c.Send(testEvent); err != nil {
		return fmt.Errorf("curl webhook 测试失败: %w", err)
	}

	return nil
}

// formatLoginMessage formats the login event message
func formatLoginMessage(event LoginEvent) string {
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

	return fmt.Sprintf(`服务器登录提醒
事件类型: %s
服务器: %s
用户: %s
来源IP: %s
来源端口: %s
认证方式: %s
位置: %s
时间: %s
日志路径: %s
日志: %s`,
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
}

// configureCurl 配置基于 curl 命令的通知
func configureCurl(curlCmd, name string) error {
	// 解析并验证 curl 命令
	notifier, err := NewCurlNotifier(curlCmd)
	if err != nil {
		return err
	}

	// 测试 curl
	fmt.Println("正在测试 curl 命令...")
	if err := notifier.Test(); err != nil {
		return err
	}
	fmt.Println("✓ 测试成功")

	// 如果未指定 name，生成默认名称
	if name == "" {
		name = generateChannelName("curl")
	}

	// 创建渠道配置
	channel := ChannelConfig{
		Name:    name,
		Enabled: true,
		Type:    "curl",
		Curl:    &CurlConfig{Command: curlCmd},
	}

	return addOrUpdateChannel(channel)
}

// addOrUpdateChannel 添加或更新渠道配置（按 Name 判断是否为同一渠道）
func addOrUpdateChannel(newChannel ChannelConfig) error {
	cm := NewConfigManager()

	// 加载现有配置
	cfg, err := cm.LoadConfig()
	if err != nil {
		if err != ErrConfigNotFound {
			return fmt.Errorf("加载配置失败: %w", err)
		}
		cfg = &Config{}
	}

	// 按 Name 查找是否已存在，存在则更新
	found := false
	for i, ch := range cfg.Channels {
		if ch.Name != "" && ch.Name == newChannel.Name {
			cfg.Channels[i] = newChannel
			found = true
			fmt.Printf("✓ 已更新渠道: %s\n", newChannel.Name)
			break
		}
	}

	// 如果不存在则添加
	if !found {
		cfg.Channels = append(cfg.Channels, newChannel)
		fmt.Printf("✓ 已添加渠道: %s\n", newChannel.Name)
	}

	// 备份并保存
	if cm.configExists() {
		if err := cm.BackupConfig(); err != nil {
			return fmt.Errorf("备份配置失败: %w", err)
		}
	}

	if err := cm.SaveConfig(*cfg); err != nil {
		if restoreErr := cm.RestoreConfig(); restoreErr != nil {
			return fmt.Errorf("保存失败且恢复失败: %v (原始错误: %w)", restoreErr, err)
		}
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Println("✓ 配置已保存")
	return nil
}

// channelDisplayName 获取渠道显示名称
func channelDisplayName(ch ChannelConfig) string {
	if ch.Name != "" {
		return ch.Name
	}
	return ch.Type
}

// generateChannelName 生成默认渠道名称
func generateChannelName(prefix string) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return fmt.Sprintf("%s_%s", prefix, string(b))
}
