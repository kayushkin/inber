// Package context implements a tag-based context system for AI agent interactions.
//
// This package replaces the traditional system-prompt/workspace/memory split with
// a unified chunk store where every piece of context (files, messages, tool results,
// system prompts) is a tagged chunk competing for context space based on relevance.
//
// # Core Concepts
//
// Chunk: A piece of context with text, tags, token count, and metadata.
//
// Store: Thread-safe in-memory storage for chunks with CRUD operations and tag-based queries.
//
// Builder: Assembles context for API calls by selecting chunks that fit within a token budget,
// prioritizing based on tags, size, and recency.
//
// Tagger: Pattern-based auto-tagging that identifies errors, code, file paths, and more.
//
// Chunker: Splits large inputs into manageable pieces while preserving tags.
//
// FileLoader: Loads files from a workspace directory as chunks with auto-generated tags
// based on file type, extension, and content.
//
// # Tag-Based Retrieval
//
// Context building follows these priorities:
//   1. Always-include chunks (tagged "identity" or "always")
//   2. Tag-matched chunks (based on message tags and chunk size)
//   3. Recent conversation chunks
//
// # Size-Aware Filtering
//
// The builder applies different tag-matching thresholds based on chunk size:
//   - < 500 tokens: include if any tag matches
//   - 500-5000 tokens: require 2+ matching tags
//   - > 5000 tokens: require 3+ matching tags
//
// # File Integration
//
// Files are first-class chunks:
//   - Each file gets tags: "file", "filename:X", extension, and category (code/config/doc)
//   - Test files (tagged "test") are excluded by default unless message includes "test" tag
//   - Respects .gitignore patterns
//   - Skips binary files and hidden files/directories
//
// # Example Usage
//
//	// Create store and load files
//	store := context.NewStore()
//	loader, _ := context.NewFileLoader(".", context.NewPatternTagger())
//	loader.LoadAndUpdate(store)
//
//	// Add identity chunk
//	store.Add(context.Chunk{
//		ID:     "identity",
//		Text:   "You are a helpful coding assistant.",
//		Tags:   []string{"identity", "always"},
//		Source: "system",
//	})
//
//	// Add user message
//	userMsg := "Fix the bug in main.go"
//	tags := context.AutoTag(userMsg, "user")
//	store.Add(context.Chunk{
//		ID:     "msg-1",
//		Text:   userMsg,
//		Tags:   tags,
//		Source: "user",
//	})
//
//	// Build context within budget
//	builder := context.NewBuilder(store, 100_000) // 100k token budget
//	chunks := builder.Build(tags)
//
//	// chunks now contains relevant context for the API call
package context
