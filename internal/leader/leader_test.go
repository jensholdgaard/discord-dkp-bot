package leader

import (
	"os"
	"testing"
)

func TestIdentity_FromPodName(t *testing.T) {
	t.Setenv("POD_NAME", "dkpbot-abc123")
	if got := identity(); got != "dkpbot-abc123" {
		t.Errorf("identity() = %q, want %q", got, "dkpbot-abc123")
	}
}

func TestIdentity_Hostname(t *testing.T) {
	t.Setenv("POD_NAME", "")
	host, err := os.Hostname()
	if err != nil {
		t.Skip("cannot get hostname")
	}
	if got := identity(); got != host {
		t.Errorf("identity() = %q, want %q", got, host)
	}
}
