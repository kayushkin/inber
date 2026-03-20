// Package memory provides persistent, searchable memory across sessions.
//
// The memory system supports:
// - Persistent storage via SQLite with full-text search
// - Semantic search using vector embeddings
// - Memory importance and decay over time
// - Session tracking and memory usage analytics
// - Memory compaction for old, unused memories
// - Lazy loading for file-based references
//
// The package is split into focused modules:
// - memory_types.go: Type definitions (Memory, Store, etc.)
// - memory_store.go: Core CRUD operations (Save, Get, Close, etc.)
// - memory_search.go: Search functionality with similarity scoring
// - memory_management.go: Memory lifecycle (Compact, Forget, DecayImportance)
// - memory_sessions.go: Session tracking and usage analytics
// - memory_migrations.go: Database schema and migrations
// - memory_utils.go: Utility functions and path helpers
package memory