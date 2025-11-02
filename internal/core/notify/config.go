package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	systemConfigDir  = "/etc/sshield"
	configFile       = "notify.json"
	defaultStateRoot = "/var/lib/sshield"
)

func resolveConfigPath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil || userConfigDir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			return "", fmt.Errorf("failed to resolve user config directory: %w", firstErr(err, homeErr))
		}
		userConfigDir = filepath.Join(home, ".config")
	}
	return filepath.Join(userConfigDir, "sshield", configFile), nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("unknown error")
}

// ConfigManager 配置管理器
type ConfigManager struct {
	configPath string
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	path, err := resolveConfigPath()
	if err != nil {
		return &ConfigManager{
			configPath: filepath.Join(systemConfigDir, configFile),
		}
	}
	return &ConfigManager{
		configPath: path,
	}
}

// SaveConfig 保存配置
func (cm *ConfigManager) SaveConfig(cfg Config) error {
	// 验证配置
	if err := ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	dir := filepath.Dir(cm.configPath)
	perm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		perm = 0755
	}

	if err := os.MkdirAll(dir, perm); err != nil {
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
	if cfg == nil {
		fmt.Println("未找到通知配置。")
		return
	}

	fmt.Println("通知配置：")
	status := "禁用"
	if cfg.Enabled {
		status = "启用"
	}
	fmt.Printf("  状态：%s\n", status)
	fmt.Printf("  类型：%s\n", strings.ToUpper(cfg.Type))
	switch strings.ToLower(cfg.Type) {
	case "webhook":
		fmt.Printf("  Webhook：%s\n", cfg.WebhookURL)
	case "email":
		fmt.Printf("  收件人：%s\n", cfg.EmailTo)
		fmt.Printf("  发件人：%s\n", cfg.EmailFrom)
		fmt.Printf("  SMTP：%s:%d\n", cfg.SMTPServer, cfg.SMTPPort)
	default:
		fmt.Println("  未知类型。")
	}
}
