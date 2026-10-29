package server

import (
	"os"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// TestMain arms the egress redactor, as cmd/inber-server does at start, so
// tests that build provider clients get a gate instead of a panic. It arms it
// with no environment: no test here may depend on a secret in the environment.
//
// When the session MCP tests start this binary as their fake MCP server
// (session_mcp_test.go), it serves MCP and runs no tests.
func TestMain(m *testing.M) {
	if os.Getenv(fakeMCPServerEnvironmentVariable) == "1" {
		runFakeMCPServer()
		return
	}
	agent.ArmEgressRedactor(nil, agent.ProviderCredentials{})
	os.Exit(m.Run())
}
