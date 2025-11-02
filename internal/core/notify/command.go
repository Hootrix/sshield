package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "管理登录通知配置",
	}

	cmd.AddCommand(
		newWebhookCmd(),
		newEmailCmd(),
		newTestCmd(),
		newStatusCmd(),
	)

	return cmd
}

func newWebhookCmd() *cobra.Command {
	var url string
	var insecure bool

	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "配置 Webhook 通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureWebhook(url, insecure)
		},
	}

	cmd.Flags().StringVarP(&url, "url", "u", "", "Webhook 目标地址")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "允许使用不安全的 HTTP Webhook（不推荐）")
	_ = cmd.MarkFlagRequired("url")

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
	)

	cmd := &cobra.Command{
		Use:   "email",
		Short: "配置 SMTP 邮件通知",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := EmailConfig{
				To:     to,
				From:   from,
				Server: server,
				User:   user,
				Pass:   pass,
				Port:   port,
			}
			return configureEmail(cfg)
		},
	}

	cmd.Flags().StringVarP(&to, "to", "t", "", "收件人邮箱地址")
	cmd.Flags().StringVarP(&from, "from", "f", "", "发件人邮箱地址")
	cmd.Flags().StringVar(&server, "server", "", "SMTP 服务器主机名")
	cmd.Flags().StringVarP(&user, "user", "u", "", "SMTP 用户名")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "SMTP 密码")
	cmd.Flags().IntVar(&port, "port", 587, "SMTP 服务器端口")

	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("server")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("password")

	return cmd
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
				DisplayLoc:   loc,
			}
			return RunSweep(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&stateFile, "state-file", "", "保存日志游标的路径（默认自动选择）")
	cmd.Flags().DurationVar(&since, "since", 1*time.Hour, "检查时间范围（默认 1h）")
	cmd.Flags().StringVar(&source, "source", "auto", "事件来源：auto｜journal｜file（默认 auto）")
	cmd.Flags().StringSliceVar(&units, "journal-unit", nil, "需要扫描的 Journal 单元名（可重复，默认 sshd.service｜ssh.service）")
	cmd.Flags().StringSliceVar(&logs, "log-path", nil, "需要扫描的 SSH 认证日志路径（可重复，默认 /var/log/auth.log、/var/log/secure）")
	cmd.Flags().StringVar(&timezone, "timezone", "Asia/Shanghai", "显示使用的时区（示例：'Asia/Shanghai'｜'Local'，默认 Asia/Shanghai）")

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
		return time.Local, nil
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
