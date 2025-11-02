package notify

import "testing"

func TestNeedsImplicitTLS(t *testing.T) {
	tests := []struct {
		port int
		want bool
	}{
		{25, false},
		{587, false},
		{465, true},
		{2525, false},
	}

	for _, tc := range tests {
		if got := needsImplicitTLS(tc.port); got != tc.want {
			t.Fatalf("needsImplicitTLS(%d) = %v want %v", tc.port, got, tc.want)
		}
	}
}

func TestValidateSMTPLine(t *testing.T) {
	if err := validateSMTPLine("normal@example.com"); err != nil {
		t.Fatalf("expected valid line, got %v", err)
	}

	if err := validateSMTPLine("bad\nline"); err == nil {
		t.Fatalf("expected error for newline")
	}
}
