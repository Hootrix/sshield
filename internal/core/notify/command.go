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
	)

	return cmd
}

func newCurlCmd() *cobra.Command {
	var isBase64 bool

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

示例：
  # 直接输入 curl 命令
  sshield notify curl 'curl -X POST -H "Content-Type: application/json" -d "{\"user\":\"{{.User}}\"}" https://example.com/webhook'

  # 使用 base64 编码（避免引号和空格问题）
  # 先编码: echo -n 'curl -X POST ...' | base64
  sshield notify curl --base64 'Y3VybCAtWCBQT1NUIC1IICJDb250ZW50LVR5cGU6IGFwcGxpY2F0aW9uL2pzb24iIC1kICJ7XCJ1c2VyXCI6XCJ7ey5Vc2VyfX1cIn0iIGh0dHBzOi8vZXhhbXBsZS5jb20vd2ViaG9vaw=='`,
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

			return configureCurl(curlCmd)
		},
	}

	cmd.Flags().BoolVar(&isBase64, "base64", false, "curl 命令使用 base64 编码")

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
