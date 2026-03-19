package main

import (
	"testing"
)

// This file serves as the main test file that imports shared test helpers.
// Individual test categories have been split into focused files:
// - cli_repo_test.go: Repository-related tests
// - cli_agents_test.go: Agent management tests
// - cli_models_test.go: Model listing tests
// - cli_sessions_test.go: Session management tests
// - cli_memory_test.go: Memory operations tests
// - cli_config_test.go: Configuration tests
// - cli_text_test.go: Text processing tests
// - cli_engine_test.go: Engine/system prompt tests
// - cli_shared_test.go: Shared test helpers

// TestMain validates that the test structure is correctly split
func TestMain(m *testing.M) {
	// Run the tests
	m.Run()
}