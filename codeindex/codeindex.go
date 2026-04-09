// Package codeindex provides AST-based codebase analysis for intelligent
// context building. It extracts symbol definitions (functions, types, interfaces)
// and builds a searchable index that feeds into the engine's prompt assembly.
//
// Inspired by Aider's repo-map and Cline's codebase indexing:
//   - Tree-sitter parsing for multi-language symbol extraction
//   - Embedding-based semantic search over symbols
//   - Token-budgeted context: signatures over full implementations
//   - Graph-based relevance ranking (most-referenced symbols first)
//
// Usage:
//
//	idx, _ := codeindex.Open(repoRoot)
//	idx.Refresh()  // re-scan changed files
//	symbols := idx.Search("authentication handler", budget: 2000)
//	// → returns ranked symbols fitting within 2000 tokens
//
// The index is stored alongside the memory DB and refreshed incrementally
// when files change (detected via mtime).
package codeindex

// Symbol represents an extracted code symbol (function, type, interface, etc.)
type Symbol struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"` // "function", "type", "interface", "method", "const", "var"
	Signature  string   `json:"signature"` // e.g. "func NewEngine(cfg EngineConfig) (*Engine, error)"
	FilePath   string   `json:"file_path"`
	Line       int      `json:"line"`
	Package    string   `json:"package"`
	References int      `json:"references"` // how many other symbols reference this one
	Tokens     int      `json:"tokens"`     // estimated token count of the signature
}

// SearchResult is a ranked symbol matching a query within a token budget.
type SearchResult struct {
	Symbol Symbol  `json:"symbol"`
	Score  float64 `json:"score"` // relevance score (0-1)
}

// Index is the codebase symbol index.
type Index struct {
	repoRoot string
	symbols  []Symbol
	// TODO: add embedding store, file mtime cache, tree-sitter parsers
}

// Open loads or creates a codebase index for the given repo root.
// Returns nil if the repo has no indexable files.
func Open(repoRoot string) (*Index, error) {
	// TODO: implement
	return &Index{repoRoot: repoRoot}, nil
}

// Refresh re-scans files that changed since the last index.
// Uses file mtime to detect changes, only re-parses what's needed.
func (idx *Index) Refresh() error {
	if idx == nil {
		return nil
	}
	// TODO: implement incremental tree-sitter parsing
	return nil
}

// Search finds symbols matching a query within a token budget.
// Results are ranked by relevance (embedding similarity × reference count).
func (idx *Index) Search(query string, tokenBudget int) []SearchResult {
	if idx == nil {
		return nil
	}
	// TODO: implement embedding search + token-budgeted ranking
	return nil
}

// RepoMap generates a concise repository map showing package structure
// and top-level symbols, similar to Aider's repo-map format.
// The map fits within the given token budget.
func (idx *Index) RepoMap(tokenBudget int) string {
	if idx == nil {
		return ""
	}
	// TODO: implement — tree of packages with key type/function signatures
	return ""
}

// Close releases index resources.
func (idx *Index) Close() error {
	return nil
}
