package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = "/etc/sshield"
	configFile = "notify.json"
)

// ConfigManager 配置管理器
type ConfigManager struct {
	configPath string
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configPath: filepath.Join(configDir, configFile),
	}
}

// SaveConfig 保存配置
func (cm *ConfigManager) SaveConfig(cfg Config) error {
	// 验证配置
	if err := ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// 确保配置目录存在
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 将配置写入文件
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
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
	// 确保配置目录存在
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config directory failed: %v", err)
	}

	// 将配置写入文件
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config failed: %v", err)
	}

	configPath := filepath.Join(configDir, configFile)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config file failed: %v", err)
	}

	return nil
}

func loadConfig() (Config, error) {
	var cfg Config
	configPath := filepath.Join(configDir, configFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config file failed: %v", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config failed: %v", err)
	}

	return cfg, nil
}

func testNotification() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config failed: %v", err)
	}

	if !cfg.Enabled {
		return fmt.Errorf("notification is not enabled")
	}

	var notifier Notifier
	switch cfg.Type {
	case "webhook":
		notifier = NewWebhookNotifier(cfg.WebhookURL)
	case "email":
		notifier = NewEmailNotifier(cfg)
	default:
		return fmt.Errorf("unknown notification type: %s", cfg.Type)
	}

	return notifier.Test()
}
