package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Hootrix/sshield/internal/core/ssh"
	"github.com/Hootrix/sshield/internal/core/firewall"
	"github.com/Hootrix/sshield/internal/core/notify"
	"github.com/Hootrix/sshield/internal/version"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:     "sshield",
	Short:   "SSHield - Linux server security configuration tool",
	Version: version.Version,
}

func init() {
	rootCmd.AddCommand(
		ssh.NewCommand(),
		firewall.NewCommand(),
		notify.NewCommand(),
	)
}
