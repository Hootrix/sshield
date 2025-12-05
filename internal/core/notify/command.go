package notify

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	envEmailToKey     = "SSHIELD_NOTIFY_EMAIL_TO"
	envEmailFromKey   = "SSHIELD_NOTIFY_EMAIL_FROM"
	envEmailServerKey = "SSHIELD_NOTIFY_EMAIL_SERVER"
	envEmailUserKey   = "SSHIELD_NOTIFY_EMAIL_USER"
	envEmailPassKey   = "SSHIELD_NOTIFY_EMAIL_PASSWORD"
	envEmailPortKey   = "SSHIELD_NOTIFY_EMAIL_PORT"
)

var (
	runSweepFunc = RunSweep
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "管理登录通知配置",
	}

	cmd.AddCommand(
		newCurlCmd(),
		newEmailCmd(),
		newTestCmd(),
		newStatusCmd(),
		newDeleteCmd(),
		newEnableCmd(),
		newDisableCmd(),
	)

	return cmd
}

func newCurlCmd() *cobra.Command {
	var (
		isBase64 bool
		name     string
	)

	cmd := &cobra.Command{
		Use:   "curl <curl命令>",
		Short: "配置 Curl 通知",
		Long: `配置基于 curl 命令的通知，支持以下模板变量：

  {{.Type}}      - 事件类型（login_success/login_failed）
  {{.User}}      - 登录用户名
  {{.IP}}        - 来源 IP
  {{.Port}}      - 来源端口
  {{.Method}}    - 认证方式（password/publickey）
  {{.Hostname}}  - 服务器主机名
  {{.Timestamp}} - 事件时间
  {{.Location}}  - IP 地理位置
  {{.LogPath}}   - 日志来源路径
  {{.Message}}   - 原始日志消息
  {{.HostIP}}    - 主机 IP

示例：
  # 直接输入 curl 命令（自动生成名称）
  sshield notify curl 'curl -X POST ...'

  # 指定渠道名称（同名则更新，不同名则新增）
  sshield notify curl --name my-webhook 'curl -X POST ...'

  # 使用 base64 编码（避免引号和空格问题）
  sshield notify curl --base64 'Y3VybCAtWCBQT1NU...'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			curlCmd := args[0]

			// 如果是 base64 编码，先解码
			if isBase64 {
				decoded, err := base64.StdEncoding.DecodeString(curlCmd)
				if err != nil {
					return fmt.Errorf("base64 解码失败: %w", err)
				}
				curlCmd = string(decoded)
				fmt.Printf("已解码 curl 命令: %s\n", curlCmd)
			}

			return configureCurl(curlCmd, name)
		},
	}

	cmd.Flags().BoolVar(&isBase64, "base64", false, "curl 命令使用 base64 编码")
	cmd.Flags().StringVar(&name, "name", "", "渠道名称（同名则更新，不指定则自动生成）")

	return cmd
}

func newEmailCmd() *cobra.Command {
	var (
		to     string
		from   string
		server string
		user   string
		pass   string
		port   int
		name   string
		envErr error
	)

	cmd := &cobra.Command{
		Use:   "email",
		Short: "配置 SMTP 邮件通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			if envErr != nil {
				return envErr
			}
			input := EmailInput{
				Name:   name,
				To:     to,
				From:   from,
				Server: server,
				User:   user,
				Pass:   pass,
				Port:   port,
			}
			return configureEmail(input)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "渠道名称（同名则更新，不指定则自动生成）")
	cmd.Flags().StringVarP(&to, "to", "t", "", "收件人邮箱地址")
	cmd.Flags().StringVarP(&from, "from", "f", "", "发件人邮箱地址")
	cmd.Flags().StringVar(&server, "server", "", "SMTP 服务器主机名")
	cmd.Flags().StringVarP(&user, "user", "u", "", "SMTP 用户名")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "SMTP 密码")
	cmd.Flags().IntVar(&port, "port", 587, "SMTP 服务器端口")

	envErr = applyEmailEnvDefaults(cmd)

	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("server")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

type envBinding struct {
	flag string
	env  string
	mask bool
}

func applyEmailEnvDefaults(cmd *cobra.Command) error {
	bindings := []envBinding{
		{flag: "to", env: envEmailToKey},
		{flag: "from", env: envEmailFromKey},
		{flag: "server", env: envEmailServerKey},
		{flag: "user", env: envEmailUserKey},
		{flag: "password", env: envEmailPassKey, mask: true},
		{flag: "port", env: envEmailPortKey},
	}

	for _, binding := range bindings {
		raw := strings.TrimSpace(os.Getenv(binding.env))
		if raw == "" {
			continue
		}

		if err := cmd.Flags().Set(binding.flag, raw); err != nil {
			return fmt.Errorf("failed to apply %s: %w", binding.env, err)
		}

		if binding.mask {
			debugf("notify: 使用环境变量 %s 覆盖 --%s", binding.env, binding.flag)
			continue
		}

		debugf("notify: 使用环境变量 %s=%s 覆盖 --%s", binding.env, raw, binding.flag)
	}

	return nil
}

// NewWatchCommand 返回 watch 子命令，用于持续监控 SSH 登录事件。
func NewWatchCommand() *cobra.Command {
	var (
		stateFile string
		poll      time.Duration
		source    string
		units     []string
		logs      []string
		timezone  string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "持续监控 SSH 登录并发送通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stateFile == "" {
				var err error
				stateFile, err = defaultCursorPath()
				if err != nil {
					return err
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				cancel()
			}()

			loc, err := resolveLocation(timezone)
			if err != nil {
				return err
			}

			opts := WatchOptions{
				CursorPath:   stateFile,
				PollTimeout:  poll,
				Source:       source,
				JournalUnits: units,
				LogPaths:     logs,
				DisplayLoc:   loc,
			}
			return RunWatch(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&stateFile, "state-file", "", "保存日志游标的路径（默认自动选择）")
	cmd.Flags().DurationVar(&poll, "poll", 5*time.Second, "等待新日志事件的超时时间（默认 5s）")
	cmd.Flags().StringVar(&source, "source", "auto", "事件来源：auto｜journal｜file（默认 auto）")
	cmd.Flags().StringSliceVar(&units, "journal-unit", nil, "需要监听的 Journal 单元名（可重复，默认 sshd.service｜ssh.service）")
	cmd.Flags().StringSliceVar(&logs, "log-path", nil, "需要跟踪的认证日志路径（可重复，默认 /var/log/auth.log、/var/log/secure）")
	cmd.Flags().StringVar(&timezone, "timezone", "Asia/Shanghai", "显示使用的时区（示例：'Asia/Shanghai'｜'Local'，默认 Asia/Shanghai）")

	return cmd
}

// NewSweepCommand 返回 sweep 子命令，用于一次性扫描 SSH 登录事件。
func NewSweepCommand() *cobra.Command {
	var (
		stateFile string
		since     time.Duration
		source    string
		units     []string
		logs      []string
		timezone  string
		notify    bool
	)

	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "单次检查最近SSH登录",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stateFile == "" {
				var err error
				stateFile, err = defaultCursorPath()
				if err != nil {
					return err
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			loc, err := resolveLocation(timezone)
			if err != nil {
				return err
			}

			opts := SweepOptions{
				CursorPath:   stateFile,
				Since:        since,
				Source:       source,
				JournalUnits: units,
				LogPaths:     logs,
				Notify:       notify,
				DisplayLoc:   loc,
			}
			return runSweepFunc(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&stateFile, "state-file", "", "保存日志游标的路径（默认自动选择）")
	cmd.Flags().DurationVar(&since, "since", 1*time.Hour, "检查时间范围（默认 1h）")
	cmd.Flags().StringVar(&source, "source", "auto", "事件来源：auto｜journal｜file（默认 auto）")
	cmd.Flags().StringSliceVar(&units, "journal-unit", nil, "需要扫描的 Journal 单元名（可重复，默认 sshd.service｜ssh.service）")
	cmd.Flags().StringSliceVar(&logs, "log-path", nil, "需要扫描的 SSH 认证日志路径（可重复，默认 /var/log/auth.log、/var/log/secure）")
	cmd.Flags().StringVar(&timezone, "timezone", "Asia/Shanghai", "显示使用的时区（示例：'Asia/Shanghai'｜'Local'，默认 Asia/Shanghai）")
	cmd.Flags().BoolVar(&notify, "notify", false, "是否发送通知（默认仅输出到控制台）")

	return cmd
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "使用当前配置发送测试通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			return testNotification()
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看当前通知配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				if errors.Is(err, ErrConfigNotFound) || errors.Is(err, ErrNotEnabled) {
					fmt.Println("通知未配置：ssh 登录事件将仅记录到日志。")
					return nil
				}
				return err
			}
			printConfigSummary(cfg)
			return nil
		},
	}
}

func newDeleteCmd() *cobra.Command {
	var (
		deleteAll   bool
		channelType string
		index       int
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "删除通知渠道配置",
		Long: `删除通知渠道配置

示例：
  # 删除所有配置
  sshield notify delete --all

  # 按类型删除（删除所有 curl 类型渠道）
  sshield notify delete --type curl

  # 按序号删除（序号可通过 status 命令查看）
  sshield notify delete --index 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !deleteAll && channelType == "" && index == 0 {
				return fmt.Errorf("请指定删除方式：--all、--type 或 --index")
			}

			if deleteAll {
				cm := NewConfigManager()
				if err := cm.DeleteConfig(); err != nil {
					return err
				}
				fmt.Println("✓ 已删除所有通知配置")
				return nil
			}

			cfg, err := loadConfig()
			if err != nil {
				if errors.Is(err, ErrConfigNotFound) {
					fmt.Println("通知未配置，无需删除。")
					return nil
				}
				return err
			}

			if channelType != "" {
				// 按类型删除
				channelType = strings.ToLower(channelType)
				var remaining []ChannelConfig
				deleted := 0
				for _, ch := range cfg.Channels {
					if strings.ToLower(ch.Type) != channelType {
						remaining = append(remaining, ch)
					} else {
						deleted++
					}
				}
				if deleted == 0 {
					return fmt.Errorf("未找到类型为 %q 的渠道", channelType)
				}
				cfg.Channels = remaining
				if err := saveConfig(*cfg); err != nil {
					return err
				}
				fmt.Printf("✓ 已删除 %d 个 %s 类型渠道\n", deleted, channelType)
				return nil
			}

			if index > 0 {
				// 按序号删除（1-indexed）
				if index > len(cfg.Channels) {
					return fmt.Errorf("序号 %d 超出范围（共 %d 个渠道）", index, len(cfg.Channels))
				}
				deleted := cfg.Channels[index-1]
				cfg.Channels = append(cfg.Channels[:index-1], cfg.Channels[index:]...)
				if err := saveConfig(*cfg); err != nil {
					return err
				}
				name := deleted.Type
				if deleted.Name != "" {
					name = deleted.Name
				}
				fmt.Printf("✓ 已删除渠道 [%d] %s\n", index, name)
				return nil
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteAll, "all", false, "删除所有通知配置")
	cmd.Flags().StringVar(&channelType, "type", "", "按类型删除（curl/email）")
	cmd.Flags().IntVar(&index, "index", 0, "按序号删除（从 1 开始，可通过 status 查看）")

	return cmd
}

func newEnableCmd() *cobra.Command {
	return newToggleCmd("enable", true)
}

func newDisableCmd() *cobra.Command {
	return newToggleCmd("disable", false)
}

func newToggleCmd(action string, enabled bool) *cobra.Command {
	var (
		name  string
		index int
		all   bool
	)

	actionCN := "启用"
	if !enabled {
		actionCN = "禁用"
	}

	cmd := &cobra.Command{
		Use:   action,
		Short: fmt.Sprintf("%s通知渠道", actionCN),
		Long: fmt.Sprintf(`%s通知渠道

示例：
  # %s所有渠道
  sshield notify %s --all

  # 按名称%s
  sshield notify %s --name my-webhook

  # 按序号%s（序号可通过 status 命令查看）
  sshield notify %s --index 1`, actionCN, actionCN, action, actionCN, action, actionCN, action),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && name == "" && index == 0 {
				return fmt.Errorf("请指定%s方式：--all、--name 或 --index", actionCN)
			}

			cfg, err := loadConfig()
			if err != nil {
				if errors.Is(err, ErrConfigNotFound) {
					return fmt.Errorf("通知未配置")
				}
				return err
			}

			if len(cfg.Channels) == 0 {
				return fmt.Errorf("没有配置任何渠道")
			}

			count := 0
			if all {
				for i := range cfg.Channels {
					if cfg.Channels[i].Enabled != enabled {
						cfg.Channels[i].Enabled = enabled
						count++
					}
				}
				if count == 0 {
					fmt.Printf("所有渠道已经是%s状态\n", actionCN)
					return nil
				}
			} else if name != "" {
				found := false
				for i := range cfg.Channels {
					if cfg.Channels[i].Name == name {
						cfg.Channels[i].Enabled = enabled
						found = true
						count++
						break
					}
				}
				if !found {
					return fmt.Errorf("未找到名称为 %q 的渠道", name)
				}
			} else if index > 0 {
				if index > len(cfg.Channels) {
					return fmt.Errorf("序号 %d 超出范围（共 %d 个渠道）", index, len(cfg.Channels))
				}
				cfg.Channels[index-1].Enabled = enabled
				count++
			}

			if err := saveConfig(*cfg); err != nil {
				return err
			}

			fmt.Printf("✓ 已%s %d 个渠道\n", actionCN, count)
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, fmt.Sprintf("%s所有渠道", actionCN))
	cmd.Flags().StringVar(&name, "name", "", fmt.Sprintf("按名称%s", actionCN))
	cmd.Flags().IntVar(&index, "index", 0, fmt.Sprintf("按序号%s（从 1 开始）", actionCN))

	return cmd
}

func resolveLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "local") {
		return shanghaiLocation, nil
	}
	if strings.EqualFold(name, "utc+8") {
		return time.FixedZone("UTC+8", 8*3600), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("无法识别的时区 %q: %w", name, err)
	}
	return loc, nil
}
