package ssh

import (
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "SSH security configuration",
	}

	cmd.AddCommand(
		newKeyOnlyCmd(),
		newPasswordCmd(),
		newPortCmd(),
	)

	return cmd
}

func newKeyOnlyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "key-only",
		Short: "Configure SSH to use key authentication only",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configureKeyOnly()
		},
	}
}

func newPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "password",
		Short: "Configure SSH password security policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configurePassword()
		},
	}
}

func newPortCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "port",
		Short: "Change SSH port",
		RunE: func(cmd *cobra.Command, args []string) error {
			return changePort(port)
		},
	}
	cmd.Flags().IntVarP(&port, "port", "p", 22, "New SSH port")
	return cmd
}
