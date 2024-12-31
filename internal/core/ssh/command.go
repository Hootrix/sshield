package ssh

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"net"
)

var (
	greenStatus = color.New(color.FgGreen, color.Bold).SprintFunc()
	redStatus   = color.New(color.FgRed, color.Bold).SprintFunc()
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "SSH相关配置",
	}

	cmd.AddCommand(
		newKeyCmd(),
		newPasswordLoginCmd(),
		newChangePasswordCmd(),
		newPortCmd(),
	)

	return cmd
}

func newPasswordLoginCmd() *cobra.Command {
	var (
		enable  bool
		disable bool
	)

	cmd := &cobra.Command{
		Use:   "password-login",
		Short: "控制 SSH 密码登录",
		Long: `控制 SSH 密码登录认证方式。

用法：
  sshield ssh password-login         显示当前密码登录状态
  sshield ssh password-login --enable   启用密码登录
  sshield ssh password-login --disable  禁用密码登录

安全建议：
1. 建议禁用密码登录，使用密钥认证
2. 如需使用密码，请确保：
   - 使用强密码（至少16位，包含大小写字母、数字和特殊字符）
   - 定期更换密码
   - 限制登录失败次数

相关命令：
  sshield ssh change-password    修改用户密码
  sshield ssh key               配置密钥登录`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 检查参数冲突
			if enable && disable {
				return fmt.Errorf("--enable 和 --disable 参数不能同时使用")
			}

			// 获取当前状态
			enabled, err := GetPasswordAuthStatus()
			if err != nil {
				return err
			}

			// 如果没有参数，只显示当前状态
			if !enable && !disable {
				status := redStatus("已启用")
				if !enabled {
					status = greenStatus("已禁用✅")
				}
				fmt.Printf(">>> SSH 密码登录当前状态：%s\n", status)

				// 添加调试信息
				fmt.Println(">>> 配置详情：")
				fmt.Println(DebugSSHConfig())

				fmt.Println(">>> 可用操作：")
				if enabled {
					fmt.Println(">>> 使用 --disable 禁用密码登录")
				} else {
					fmt.Println(">>> 使用 --enable 启用密码登录")
				}
				return nil
			}

			// 确定目标状态
			targetEnabled := enable
			action := "启用"
			if disable {
				targetEnabled = false
				action = "禁用"
			}

			// 如果当前状态与目标状态相同，直接返回
			if enabled == targetEnabled {
				fmt.Printf(">>> SSH 密码登录已经%s，无需修改\n", map[bool]string{true: "启用", false: "禁用"}[enabled])
				return nil
			}

			// 显示警告信息
			if targetEnabled {
				fmt.Println(">>> 警告：启用密码登录可能会降低系统安全性。建议：")
				fmt.Println(">>> 1. 使用强密码")
				fmt.Println(">>> 2. 同时配置密钥认证")
				fmt.Println(">>> 3. 定期更换密码")
				fmt.Println()
			}

			// 确认操作
			fmt.Printf(">>> 确定要%s密码登录吗？[y/N] ", action)
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Printf(">>> 已取消%s密码登录\n", action)
				return nil
			}

			// 配置 SSH
			config := SSHAuthConfig{
				KeyType:         DefaultKeyConfig(),
				DisablePassword: !targetEnabled,
			}
			if err := ConfigureAuth(config); err != nil {
				return fmt.Errorf("%s密码登录失败: %v", action, err)
			}

			fmt.Printf("\n>>> SSH 密码登录已成功%s\n", action)
			if targetEnabled {
				fmt.Println(">>> 提示：使用 'sshield ssh change-password' 命令可以修改密码")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&enable, "enable", false, "启用密码登录")
	cmd.Flags().BoolVar(&disable, "disable", false, "禁用密码登录")
	return cmd
}

func newKeyCmd() *cobra.Command {
	var (
		keyType string
		bits    int
		email   string
	)

	cmd := &cobra.Command{
		Use:   "key",
		Short: "配置SSH密钥登录",
		Long: `配置SSH密钥登录认证方式。

用法：
  sshield ssh key                  显示当前密钥配置
  sshield ssh key --type ed25519   使用 ED25519 密钥（推荐）
  sshield ssh key --type rsa       使用 RSA 密钥
  
可选参数：
  --type              密钥类型，可选 ed25519（推荐）或 rsa
  --bits             RSA密钥长度，仅当 type=rsa 时有效，默认 4096
  --email            密钥注释，通常使用邮箱，默认使用当前用户名

安全建议：
1. 推荐使用 ED25519 密钥，更安全且性能更好
2. 如果必须使用 RSA，建议密钥长度至少 4096 位
3. 建议同时禁用密码登录，仅使用密钥认证
4. 请妥善保管私钥，建议设置密钥密码`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 如果没有参数，显示当前配置
			if !cmd.Flags().Changed("type") {
				fmt.Println(">>> 当前SSH密钥配置：")

				// 检查是否存在密钥
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("获取用户目录失败: %v", err)
				}

				keyTypes := []string{"ed25519", "rsa"}
				foundKey := false

				for _, kt := range keyTypes {
					keyPath := fmt.Sprintf("%s/.ssh/id_%s.pub", homeDir, kt)
					if _, err := os.Stat(keyPath); err == nil {
						foundKey = true
						content, err := os.ReadFile(keyPath)
						if err != nil {
							continue
						}
						fmt.Printf(">>> %s 密钥：%s\n", strings.ToUpper(kt), keyPath)
						parts := strings.Fields(string(content))
						if len(parts) >= 3 {
							fmt.Printf(">>>     指纹：%s\n", parts[1][:16]+"...")
							fmt.Printf(">>>     注释：%s\n", parts[2])
						}
					}
				}

				if !foundKey {
					fmt.Println(">>> 未找到任何SSH密钥")
					fmt.Println(">>>     使用 --type 参数生成新密钥")
				}

				// 显示认证状态
				pubkeyEnabled, _ := GetPubKeyAuthStatus()
				pubkeyStatus := redStatus("已禁用")
				if pubkeyEnabled {
					pubkeyStatus = greenStatus("已启用✅")
				}
				fmt.Printf(">>> 密钥认证：%s\n", pubkeyStatus)

				// 显示密码登录状态
				passwordEnabled, _ := GetPasswordAuthStatus()
				passwordStatus := greenStatus("已禁用✅")
				if passwordEnabled {
					passwordStatus = redStatus("已启用")
				}
				fmt.Printf(">>> 密码登录：%s\n", passwordStatus)

				// 显示配置详情
				fmt.Println(">>> 配置详情：")
				fmt.Println(DebugSSHConfig())

				return nil
			}

			// 验证密钥类型
			var config SSHAuthConfig
			switch KeyType(keyType) {
			case KeyTypeEd25519:
				config.KeyType = KeyTypeConfig{
					Type: KeyTypeEd25519,
				}
			case KeyTypeRSA:
				if bits < 2048 {
					return fmt.Errorf("RSA密钥长度必须大于等于2048位")
				}
				config.KeyType = KeyTypeConfig{
					Type: KeyTypeRSA,
					Bits: bits,
				}
			default:
				return fmt.Errorf("不支持的密钥类型：%s", keyType)
			}

			// 如果未指定邮箱，使用当前用户名
			if email == "" {
				currentUser, err := user.Current()
				if err != nil {
					return fmt.Errorf("获取当前用户失败: %v", err)
				}
				email = currentUser.Username
			}

			// 配置SSH
			if err := prepareKeyAuth(email, config.KeyType); err != nil {
				return fmt.Errorf("配置密钥失败: %v", err)
			}

			// 如果需要禁用密码登录
			if config.DisablePassword {
				if err := ConfigureAuth(config); err != nil {
					return fmt.Errorf("禁用密码登录失败: %v", err)
				}
			}

			fmt.Printf("\n>>> SSH密钥配置完成！\n")
			fmt.Println(">>> 后续步骤：")
			fmt.Println(">>> 1. 私钥路径：~/.ssh/id_" + keyType)
			fmt.Println(">>> 2. 公钥路径：~/.ssh/id_" + keyType + ".pub")
			if config.DisablePassword {
				fmt.Println(">>> 3. 密码登录已禁用，请确保密钥配置正确")
			} else {
				fmt.Println(">>> 3. 建议使用禁用密码登录")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&keyType, "type", "", "密钥类型：ed25519 或 rsa")
	cmd.Flags().IntVar(&bits, "bits", 4096, "RSA密钥长度")
	cmd.Flags().StringVar(&email, "email", "", "密钥注释（通常使用邮箱）")

	return cmd
}

func newChangePasswordCmd() *cobra.Command {
	var (
		username       string
		useRandom      bool
		passwordLength int
	)

	cmd := &cobra.Command{
		Use:   "change-password",
		Short: "修改用户密码",
		Long: `修改指定用户的密码。
如果不指定用户名，则修改当前用户的密码。
修改完成后无需重启 SSH 服务。

选项：
  -u, --user string    要修改密码的用户名（默认为当前用户）
  -r, --random        使用随机生成的强密码
  -l, --length int    随机密码的长度（默认20位，最小16位）

密码要求：
1. 长度至少16位
2. 必须包含大写字母、小写字母、数字和特殊字符

示例：
  # 修改当前用户密码
  sshield ssh change-password

  # 使用随机密码
  sshield ssh change-password -r

  # 为指定用户生成20位随机密码
  sshield ssh change-password -u username -r -l 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 如果没有指定用户名，使用当前用户
			if username == "" {
				currentUser, err := user.Current()
				if err != nil {
					return fmt.Errorf("获取当前用户失败: %v", err)
				}
				username = currentUser.Username
			}

			// 检查是否有root权限
			if os.Geteuid() != 0 {
				return fmt.Errorf("需要root权限才能修改密码")
			}

			var password string
			if useRandom {
				// 生成随机密码
				randomPass, err := generateRandomPassword(passwordLength)
				if err != nil {
					return err
				}
				password = randomPass

				// 显示生成的密码
				fmt.Printf(">>> 已生成随机密码: %s\n", password)
				fmt.Print(">>> 是否使用这个密码? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					return fmt.Errorf("已取消密码修改")
				}
			} else {
				// 手动输入密码
				fmt.Print(">>> 请输入新密码: ")
				passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
				if err != nil {
					return fmt.Errorf("读取密码失败: %v", err)
				}
				fmt.Println()

				// 确认密码
				fmt.Print(">>> 请再次输入新密码: ")
				confirmPassword, err := term.ReadPassword(int(syscall.Stdin))
				if err != nil {
					return fmt.Errorf("读取密码失败: %v", err)
				}
				fmt.Println()

				// 检查两次输入是否一致
				if string(passwordBytes) != string(confirmPassword) {
					return fmt.Errorf("两次输入的密码不一致")
				}

				password = string(passwordBytes)
			}

			// 验证密码强度
			if err := validatePassword(password); err != nil {
				return err
			}

			// 修改密码
			if err := changePassword(username, password); err != nil {
				return err
			}

			fmt.Printf(">>> 用户 %s 的密码已成功修改\n", username)
			return nil
		},
	}

	// 添加命令行参数
	cmd.Flags().StringVarP(&username, "user", "u", "", "要修改密码的用户名（默认为当前用户）")
	cmd.Flags().BoolVarP(&useRandom, "random", "r", false, "使用随机生成的强密码")
	cmd.Flags().IntVarP(&passwordLength, "length", "l", 20, "随机密码的长度（默认20位，最小16位）")

	return cmd
}

func newPortCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "port [端口号]",
		Short: "修改 SSH 端口",
		Long: `修改 SSH 服务的端口号。

参数：
  [端口号]  新的 SSH 端口号（1-65535）

选项：
  -p, --port int   新的 SSH 端口号（默认为22）

示例：
  # 使用参数修改端口
  sshield ssh port 2222

  # 使用选项修改端口
  sshield ssh port -p 2222`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 如果提供了位置参数，优先使用位置参数
			if len(args) > 0 {
				parsedPort, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("无效的端口号：%s", args[0])
				}
				port = parsedPort
			}

			// 验证端口范围
			if port < 1 || port > 65535 {
				return fmt.Errorf("端口号必须在 1-65535 之间")
			}

			// 检查端口是否被占用
			if err := checkPortAvailability(port); err != nil {
				return fmt.Errorf("端口 %d 检查失败: %v", port, err)
			}

			// 如果没有指定端口，显示当前端口
			if port == 22 && len(args) == 0 && !cmd.Flags().Changed("port") {
				currentPort, err := GetSSHPort()
				if err != nil {
					return fmt.Errorf("获取当前SSH端口失败: %v", err)
				}

				fmt.Println(">>> 当前SSH配置：")
				fmt.Printf(">>> 端口号：%d\n", currentPort)
				return nil
			}

			// 确认修改
			fmt.Printf(">>> 确定要将 SSH 端口修改为 %d 吗？[y/N] ", port)
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println(">>> 已取消端口修改")
				return nil
			}

			// 修改端口
			if err := changePort(port); err != nil {
				return err
			}

			fmt.Printf(">>> SSH 端口已成功修改为 %d\n", port)
			fmt.Printf(">>> 请确保防火墙已允许该端口访问\n")
			return nil
		},
	}
	cmd.Flags().IntVarP(&port, "port", "p", 22, "新的 SSH 端口号")
	return cmd
}

func checkPortAvailability(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	defer ln.Close()
	return nil
}
