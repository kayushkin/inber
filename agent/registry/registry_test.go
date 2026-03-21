// Package registry tests are split into focused modules for better maintainability:
//
// - registry_test_mocks.go: Mock implementations and test helpers
// - tool_registry_test.go: Tool registration and memory/spawn tools tests  
// - registry_creation_test.go: Registry creation and factory function tests
// - registry_core_test.go: Agent listing, configuration, and session management tests
package registry