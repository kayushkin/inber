// Package memory provides a thin wrapper around github.com/kayushkin/memory-store.
// This package re-exports its types and interfaces while maintaining the same API that
// inber expects. The store was split out of agent-store into its own module, and
// agent-store has had no memory subpackage since; the old path is not an alias for the
// new one, it does not resolve at all.
package memory

import (
	memorystore "github.com/kayushkin/memory-store"
)

// Re-export types from memory-store
type Memory = memorystore.Memory
type MemoryStore = memorystore.MemoryStore
type CompactionResult = memorystore.CompactionResult
type BuildContextRequest = memorystore.BuildContextRequest
type PrepareSessionConfig = memorystore.PrepareSessionConfig
type ToolMetadata = memorystore.ToolMetadata
type RecentFile = memorystore.RecentFile
type Embedder = memorystore.Embedder
type Tagger = memorystore.Tagger
type PatternTagger = memorystore.PatternTagger

// Re-export functions from memory-store
var NewStore = memorystore.NewStore
var OpenOrCreate = memorystore.OpenOrCreate
var NewSQLiteStore = memorystore.NewSQLiteStore

var DefaultPrepareSessionConfig = memorystore.DefaultPrepareSessionConfig
var FindRecentlyModified = memorystore.FindRecentlyModified
var FormatRecentFiles = memorystore.FormatRecentFiles
var NewEmbedder = memorystore.NewEmbedder
var CosineSimilarity = memorystore.CosineSimilarity
var NewPatternTagger = memorystore.NewPatternTagger
var AutoTag = memorystore.AutoTag
var EstimateTokens = memorystore.EstimateTokens
var EstimateMessageOverhead = memorystore.EstimateMessageOverhead
var EstimateToolSchemaTokens = memorystore.EstimateToolSchemaTokens
var SessionsSchema = memorystore.SessionsSchema
var DefaultMemoryPath = memorystore.DefaultMemoryPath
