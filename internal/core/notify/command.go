package notify

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Login notification configuration",
	}

	cmd.AddCommand(
		newWebhookCmd(),
		newEmailCmd(),
		newTestCmd(),
	)

	return cmd
}

func newWebhookCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Configure webhook notification",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureWebhook(url)
		},
	}
	cmd.Flags().StringVarP(&url, "url", "u", "", "Webhook URL")
	cmd.MarkFlagRequired("url")
	return cmd
}

func newEmailCmd() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Configure email notification",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureEmail(email)
		},
	}
	cmd.Flags().StringVarP(&email, "email", "e", "", "Email address")
	cmd.MarkFlagRequired("email")
	return cmd
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test notification configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return testNotification()
		},
	}
}
