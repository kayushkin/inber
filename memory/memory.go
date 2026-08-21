// Package memory provides a thin wrapper around github.com/kayushkin/memory-store.
// This package re-exports its types and interfaces while maintaining the same API
// that inber expects. The store was split out of agent-store into its own module;
// this package named the old home for as long as it took someone to grep for it.
package memory

import (
	asmemory "github.com/kayushkin/memory-store"
)

// Re-export types from agent-store/memory
type Memory = asmemory.Memory
type MemoryStore = asmemory.MemoryStore
type CompactionResult = asmemory.CompactionResult
type BuildContextRequest = asmemory.BuildContextRequest
type PrepareSessionConfig = asmemory.PrepareSessionConfig
type ToolMetadata = asmemory.ToolMetadata
type RecentFile = asmemory.RecentFile
type Embedder = asmemory.Embedder
type Tagger = asmemory.Tagger
type PatternTagger = asmemory.PatternTagger

// Re-export functions from agent-store/memory
var NewStore = asmemory.NewStore
var OpenOrCreate = asmemory.OpenOrCreate
var NewSQLiteStore = asmemory.NewSQLiteStore

var DefaultPrepareSessionConfig = asmemory.DefaultPrepareSessionConfig
var FindRecentlyModified = asmemory.FindRecentlyModified
var FormatRecentFiles = asmemory.FormatRecentFiles
var NewEmbedder = asmemory.NewEmbedder
var CosineSimilarity = asmemory.CosineSimilarity
var NewPatternTagger = asmemory.NewPatternTagger
var AutoTag = asmemory.AutoTag
var EstimateTokens = asmemory.EstimateTokens
var EstimateMessageOverhead = asmemory.EstimateMessageOverhead
var EstimateToolSchemaTokens = asmemory.EstimateToolSchemaTokens
var SessionsSchema = asmemory.SessionsSchema
var DefaultMemoryPath = asmemory.DefaultMemoryPath