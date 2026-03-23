// Package conversation provides conversation management and pruning functionality.
// This package has been split into focused modules for better maintainability:
//
// - prune_config.go: Configuration types and factory functions
// - prune_core.go:   Main PruneConversation function and result types
// - prune_utils.go:  Utility functions for text processing and token estimation
package conversation