package firewall

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall",
		Short: "Firewall configuration",
	}

	cmd.AddCommand(
		newSetupCmd(),
		newStatusCmd(),
		newRuleCmd(),
	)

	return cmd
}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Setup firewall configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup()
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show firewall status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return status()
		},
	}
}

func newRuleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rule",
		Short: "Manage firewall rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return manageRules()
		},
	}
}
