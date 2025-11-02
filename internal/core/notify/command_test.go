package notify

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyEmailEnvDefaults(t *testing.T) {
	cmd := &cobra.Command{Use: "email"}
	var (
		to     string
		from   string
		server string
		user   string
		pass   string
		port   int
	)
	cmd.Flags().StringVar(&to, "to", "", "")
	cmd.Flags().StringVar(&from, "from", "", "")
	cmd.Flags().StringVar(&server, "server", "", "")
	cmd.Flags().StringVar(&user, "user", "", "")
	cmd.Flags().StringVar(&pass, "password", "", "")
	cmd.Flags().IntVar(&port, "port", 587, "")

	t.Setenv(envEmailToKey, "ops@example.com")
	t.Setenv(envEmailFromKey, "noreply@example.com")
	t.Setenv(envEmailServerKey, "smtp.example.com")
	t.Setenv(envEmailUserKey, "smtp-user")
	t.Setenv(envEmailPassKey, "secret")
	t.Setenv(envEmailPortKey, "2525")

	if err := applyEmailEnvDefaults(cmd); err != nil {
		t.Fatalf("applyEmailEnvDefaults returned error: %v", err)
	}

	if to != "ops@example.com" {
		t.Fatalf("expected to=ops@example.com, got %s", to)
	}
	if from != "noreply@example.com" {
		t.Fatalf("expected from=noreply@example.com, got %s", from)
	}
	if server != "smtp.example.com" {
		t.Fatalf("expected server=smtp.example.com, got %s", server)
	}
	if user != "smtp-user" {
		t.Fatalf("expected user=smtp-user, got %s", user)
	}
	if pass != "secret" {
		t.Fatalf("expected password to be populated from env")
	}
	if port != 2525 {
		t.Fatalf("expected port=2525, got %d", port)
	}
}

func TestApplyEmailEnvDefaultsInvalidPort(t *testing.T) {
	cmd := &cobra.Command{Use: "email"}
	var port int
	cmd.Flags().IntVar(&port, "port", 587, "")

	t.Setenv(envEmailPortKey, "invalid")

	if err := applyEmailEnvDefaults(cmd); err == nil {
		t.Fatalf("expected error for invalid port")
	}
}
