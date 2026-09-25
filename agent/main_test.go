package agent

import (
	"os"
	"testing"
)

// TestMain arms the egress redactor, as cmd/inber-server does at start, so
// tests that build provider clients get a gate instead of a panic. It arms it
// with no environment: no test here may depend on a secret in the environment.
func TestMain(m *testing.M) {
	ArmEgressRedactor(nil, ProviderAPIKeys{})
	os.Exit(m.Run())
}
