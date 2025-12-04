package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// 配置目录
	configDir = "/etc/sshield"
	// 配置文件
	configFile = "notify.json"
	// 状态目录
	defaultStateRoot = "/var/lib/sshield"
)

func resolveConfigPath() string {
	return filepath.Join(configDir, configFile)
}

// ConfigManager 配置管理器
type ConfigManager struct {
	configPath string
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configPath: resolveConfigPath(),
	}
}

// SaveConfig 保存配置
func (cm *ConfigManager) SaveConfig(cfg Config) error {
	// 验证配置
	if err := ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 将配置写入文件
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadConfig 加载配置
func (cm *ConfigManager) LoadConfig() (*Config, error) {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 验证加载的配置
	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// BackupConfig 备份配置
func (cm *ConfigManager) BackupConfig() error {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	backupPath := cm.configPath + ".backup"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// RestoreConfig 恢复配置
func (cm *ConfigManager) RestoreConfig() error {
	backupPath := cm.configPath + ".backup"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore config file: %w", err)
	}

	return nil
}

// DeleteConfig 删除配置
func (cm *ConfigManager) DeleteConfig() error {
	if err := os.Remove(cm.configPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete config file: %w", err)
		}
	}
	return nil
}

// configExists 检查配置文件是否存在
func (cm *ConfigManager) configExists() bool {
	_, err := os.Stat(cm.configPath)
	return err == nil
}

func saveConfig(cfg Config) error {
	manager := NewConfigManager()
	return manager.SaveConfig(cfg)
}

func loadConfig() (*Config, error) {
	manager := NewConfigManager()
	return manager.LoadConfig()
}

func printConfigSummary(cfg *Config) {
	if cfg == nil || len(cfg.Channels) == 0 {
		fmt.Println("未配置通知渠道。")
		return
	}

	fmt.Printf("通知渠道（共 %d 个）：\n", len(cfg.Channels))
	for i, ch := range cfg.Channels {
		status := "禁用"
		if ch.Enabled {
			status = "启用"
		}

		name := ch.Type
		if ch.Name != "" {
			name = ch.Name
		}

		fmt.Printf("\n  [%d] %s（%s）\n", i+1, strings.ToUpper(ch.Type), status)
		if ch.Name != "" {
			fmt.Printf("      名称：%s\n", name)
		}

		switch strings.ToLower(ch.Type) {
		case "curl":
			if ch.Curl != nil {
				curlDisplay := ch.Curl.Command
				if len(curlDisplay) > 60 {
					curlDisplay = curlDisplay[:57] + "..."
				}
				fmt.Printf("      命令：%s\n", curlDisplay)
			}
		case "email":
			if ch.Email != nil {
				fmt.Printf("      收件人：%s\n", ch.Email.To)
				fmt.Printf("      发件人：%s\n", ch.Email.From)
				fmt.Printf("      SMTP：%s:%d\n", ch.Email.Server, ch.Email.Port)
			}
		}
	}
}
