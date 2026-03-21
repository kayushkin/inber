// Package memory provides a thin wrapper around github.com/kayushkin/agent-store/memory.
// This package re-exports types and interfaces from agent-store/memory while maintaining
// the same API that inber expects.
package memory

import (
	asmemory "github.com/kayushkin/agent-store/memory"
)

// Re-export types from agent-store/memory
type Memory = asmemory.Memory
type MemoryStore = asmemory.MemoryStore
type CompactionResult = asmemory.CompactionResult
type BuildContextRequest = asmemory.BuildContextRequest
type PrepareSessionConfig = asmemory.PrepareSessionConfig
type Session = asmemory.Session
type ToolMetadata = asmemory.ToolMetadata
type RecentFile = asmemory.RecentFile
type Embedder = asmemory.Embedder
type Tagger = asmemory.Tagger
type PatternTagger = asmemory.PatternTagger

// Re-export functions from agent-store/memory
var NewStore = asmemory.NewStore
var OpenOrCreate = asmemory.OpenOrCreate
var NewSQLiteStore = asmemory.NewSQLiteStore
var NewNATSStore = asmemory.NewNATSStore
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