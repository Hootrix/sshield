package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Manage login notification and monitoring",
	}

	cmd.AddCommand(
		newWebhookCmd(),
		newEmailCmd(),
		newWatchCmd(),
		newSweepCmd(),
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
		Short: "Configure webhook notification",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureWebhook(url, insecure)
		},
	}

	cmd.Flags().StringVarP(&url, "url", "u", "", "Webhook target URL")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "Allow insecure HTTP webhook (not recommended)")
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
		Short: "Configure SMTP email notification",
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

	cmd.Flags().StringVarP(&to, "to", "t", "", "Recipient email address")
	cmd.Flags().StringVarP(&from, "from", "f", "", "Sender email address")
	cmd.Flags().StringVar(&server, "server", "", "SMTP server host")
	cmd.Flags().StringVarP(&user, "user", "u", "", "SMTP username")
	cmd.Flags().StringVarP(&pass, "password", "p", "", "SMTP password")
	cmd.Flags().IntVar(&port, "port", 587, "SMTP server port")

	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("server")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

func newWatchCmd() *cobra.Command {
	var (
		stateFile string
		poll      time.Duration
		source    string
		units     []string
		logs      []string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Continuously monitor SSH logins and send notifications",
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

			opts := WatchOptions{
				CursorPath:   stateFile,
				PollTimeout:  poll,
				Source:       source,
				JournalUnits: units,
				LogPaths:     logs,
			}
			return RunWatch(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&stateFile, "state-file", "", "Path to store journal cursor (default auto)")
	cmd.Flags().DurationVar(&poll, "poll", 5*time.Second, "Journal wait timeout")
	cmd.Flags().StringVar(&source, "source", "auto", "Event source: auto|journal|file")
	cmd.Flags().StringSliceVar(&units, "journal-unit", nil, "Journal unit name to watch (repeatable)")
	cmd.Flags().StringSliceVar(&logs, "log-path", nil, "Auth log file path to follow (repeatable)")

	return cmd
}

func newSweepCmd() *cobra.Command {
	var (
		stateFile string
		since     time.Duration
		source    string
		units     []string
		logs      []string
	)

	cmd := &cobra.Command{
		Use:   "sweep",
		Short: "Process recent SSH login events once and exit",
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

			opts := SweepOptions{
				CursorPath:   stateFile,
				Since:        since,
				Source:       source,
				JournalUnits: units,
				LogPaths:     logs,
			}
			return RunSweep(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&stateFile, "state-file", "", "Path to store journal cursor (default auto)")
	cmd.Flags().DurationVar(&since, "since", 5*time.Minute, "Process events newer than duration if no cursor is stored")
	cmd.Flags().StringVar(&source, "source", "auto", "Event source: auto|journal|file")
	cmd.Flags().StringSliceVar(&units, "journal-unit", nil, "Journal unit name to scan (repeatable)")
	cmd.Flags().StringSliceVar(&logs, "log-path", nil, "Auth log file path to scan (repeatable)")

	return cmd
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Send a test notification using current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return testNotification()
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current notification configuration",
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
