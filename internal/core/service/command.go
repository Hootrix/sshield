package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	serviceName     = "sshield-notify"
	serviceFilePath = "/etc/systemd/system/sshield-notify.service"
)

// serviceTemplate 是 systemd service 文件模板
// StartLimitIntervalSec/StartLimitBurst 放在 [Unit] 段以兼容旧版 systemd (< 229)
const serviceTemplate = `[Unit]
Description=SSHield SSH login watcher
After=network.target syslog.target
StartLimitIntervalSec=60
StartLimitBurst=10

[Service]
Type=simple
ExecStart=%s ssh watch
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sshield

[Install]
WantedBy=multi-user.target
`

// NewCommand 返回 service 子命令
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "管理 sshield 系统服务",
	}

	cmd.AddCommand(
		newInstallCommand(),
		newUninstallCommand(),
		newStatusCommand(),
	)

	return cmd
}

func newInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "安装 systemd 服务（需要 root 权限）",
		Long: `安装 sshield-notify.service 到 systemd。
安装后需手动启动：
  sudo systemctl start sshield-notify
  sudo systemctl enable sshield-notify`,
		RunE: runInstall,
	}
}

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "卸载 systemd 服务（需要 root 权限）",
		RunE:  runUninstall,
	}
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看服务状态",
		RunE:  runStatus,
	}
}

func runInstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd 服务仅支持 Linux 系统")
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("需要 root 权限，请使用 sudo 运行")
	}

	if !hasSystemd() {
		return fmt.Errorf("未检测到 systemd，当前系统可能使用 SysVinit 或 OpenRC")
	}

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("解析可执行文件路径失败: %w", err)
	}

	// 生成 service 文件内容
	content := fmt.Sprintf(serviceTemplate, execPath)

	// 检查是否已存在
	if _, err := os.Stat(serviceFilePath); err == nil {
		fmt.Printf("服务文件已存在: %s\n", serviceFilePath)
		fmt.Println("如需重新安装，请先运行: sshield service uninstall")
		return nil
	}

	// 写入 service 文件
	if err := os.WriteFile(serviceFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入服务文件失败: %w", err)
	}
	fmt.Printf("✓ 已创建服务文件: %s\n", serviceFilePath)

	// 重新加载 systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 失败: %w", err)
	}
	fmt.Println("✓ 已重新加载 systemd 配置")

	fmt.Println()
	fmt.Println("服务已安装，请手动启动：")
	fmt.Printf("  sudo systemctl start %s\n", serviceName)
	fmt.Printf("  sudo systemctl enable %s  # 开机自启\n", serviceName)
	fmt.Println()
	fmt.Println("查看状态：")
	fmt.Printf("  sudo systemctl status %s\n", serviceName)
	fmt.Printf("  sudo journalctl -u %s -f\n", serviceName)

	return nil
}

func runUninstall(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd 服务仅支持 Linux 系统")
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("需要 root 权限，请使用 sudo 运行")
	}

	// 停止服务（忽略错误，可能未运行）
	_ = exec.Command("systemctl", "stop", serviceName).Run()
	_ = exec.Command("systemctl", "disable", serviceName).Run()

	// 删除 service 文件
	if err := os.Remove(serviceFilePath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("服务文件不存在，无需卸载")
			return nil
		}
		return fmt.Errorf("删除服务文件失败: %w", err)
	}

	// 重新加载 systemd
	_ = exec.Command("systemctl", "daemon-reload").Run()

	fmt.Printf("✓ 已卸载服务: %s\n", serviceName)
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd 服务仅支持 Linux 系统")
	}

	if !hasSystemd() {
		fmt.Println("未检测到 systemd")
		return nil
	}

	// 检查服务文件是否存在
	if _, err := os.Stat(serviceFilePath); os.IsNotExist(err) {
		fmt.Println("服务未安装")
		fmt.Println("运行以下命令安装：")
		fmt.Println("  sudo sshield service install")
		return nil
	}

	// 调用 systemctl status
	output, err := exec.Command("systemctl", "status", serviceName, "--no-pager").CombinedOutput()
	if err != nil {
		// systemctl status 在服务未运行时返回非零退出码，但仍有输出
		if len(output) > 0 {
			fmt.Println(string(output))
			return nil
		}
		return fmt.Errorf("获取服务状态失败: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// hasSystemd 检查系统是否使用 systemd
func hasSystemd() bool {
	// 方法1：检查 /run/systemd/system 目录
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}

	// 方法2：检查 PID 1 是否是 systemd
	if data, err := os.ReadFile("/proc/1/comm"); err == nil {
		comm := strings.TrimSpace(string(data))
		if comm == "systemd" {
			return true
		}
	}

	return false
}
