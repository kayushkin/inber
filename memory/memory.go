// Package memory provides persistent, searchable memory across sessions.
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

// Memory represents a single persistent memory entry.
type Memory struct {
	ID           string    // unique identifier
	Content      string    // the actual text
	Summary      string    // compressed version (nullable, for future compaction)
	OriginalID   string    // pointer to parent memory if compacted (nullable)
	Tags         []string  // tags for categorization
	Importance   float64   // 0-1, how important this memory is
	AccessCount  int       // how many times it's been retrieved
	LastAccessed time.Time // timestamp of last retrieval
	CreatedAt    time.Time // when it was stored
	Source       string    // "user", "agent", "reflection", "compaction", "system"
	Embedding    []float64 // vector for semantic search
	
	// New fields for context unification
	AlwaysLoad   bool       // if true, always include in context (e.g., identity)
	ExpiresAt    *time.Time // optional expiration (for ephemeral content like recent files)
	Tokens       int        // pre-computed token count for budget management
	
	// Reference fields for lazy loading
	RefType      string     // "memory" (default), "file", "identity", "repo-map", "tools", "web"
	RefTarget    string     // file path, URL, or empty for pure memories
	IsLazy       bool       // if true, load content on-demand instead of from DB
}

// Store handles persistent memory storage via SQLite.
type Store struct {
	db       *sql.DB
	embedder *Embedder
}

// NewStore creates or opens a memory store at the given path.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Create normalized schema
	// Note: We only create the table and basic indexes here
	// Migrations will add new columns to existing tables
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		summary TEXT,
		original_id TEXT,
		importance REAL NOT NULL DEFAULT 0.5,
		access_count INTEGER NOT NULL DEFAULT 0,
		last_accessed INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		source TEXT NOT NULL,
		embedding BLOB
	);
	CREATE INDEX IF NOT EXISTS idx_importance ON memories(importance);
	CREATE INDEX IF NOT EXISTS idx_last_accessed ON memories(last_accessed);
	CREATE INDEX IF NOT EXISTS idx_created_at ON memories(created_at);
	CREATE INDEX IF NOT EXISTS idx_source ON memories(source);

	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_memory_id ON memory_tags(memory_id);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);

	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Create session-related tables
	if _, err := db.Exec(SessionsSchema()); err != nil {
		db.Close()
		return nil, fmt.Errorf("create sessions schema: %w", err)
	}

	// Run migrations for existing databases
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{
		db:       db,
		embedder: NewEmbedder(),
	}, nil
}

// runMigrations applies schema migrations to existing databases
func runMigrations(db *sql.DB) error {
	// Get list of existing columns
	existingCols := make(map[string]bool)
	rows, err := db.Query("PRAGMA table_info(memories)")
	if err != nil {
		return err
	}
	defer rows.Close()
	
	var cid int
	var name, typ string
	var notnull, pk int
	var dflt sql.NullString
	
	for rows.Next() {
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existingCols[name] = true
	}
	
	// List of all migrations (idempotent - run them all, sqlite will ignore errors for existing columns)
	migrations := []string{
		// Context unification fields (2026-03-01)
		"ALTER TABLE memories ADD COLUMN always_load INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE memories ADD COLUMN expires_at INTEGER",
		"ALTER TABLE memories ADD COLUMN tokens INTEGER NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_always_load ON memories(always_load)",
		"CREATE INDEX IF NOT EXISTS idx_expires_at ON memories(expires_at)",
		
		// Reference fields for lazy loading (2026-03-01)
		"ALTER TABLE memories ADD COLUMN ref_type TEXT NOT NULL DEFAULT 'memory'",
		"ALTER TABLE memories ADD COLUMN ref_target TEXT",
		"ALTER TABLE memories ADD COLUMN is_lazy INTEGER NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_ref_type ON memories(ref_type)",
	}
	
	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			// Ignore errors for already-existing columns/indexes
			continue
		}
	}
	
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Save stores a new memory.
func (s *Store) Save(m Memory) error {
	// Generate embedding if not provided
	if len(m.Embedding) == 0 {
		m.Embedding = s.embedder.Embed(m.Content)
	}

	// Set defaults
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.LastAccessed.IsZero() {
		m.LastAccessed = m.CreatedAt
	}
	if m.Importance == 0 {
		m.Importance = 0.5
	}

	// Serialize embedding
	embJSON, err := json.Marshal(m.Embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Auto-compute tokens if not set
	if m.Tokens == 0 && m.Content != "" {
		m.Tokens = (len(m.Content) + 2) / 3 // ~3 chars per token
	}
	
	// Set default ref_type if empty
	if m.RefType == "" {
		m.RefType = "memory"
	}

	// Upsert memory (insert or update on conflict)
	query := `
	INSERT INTO memories (id, content, summary, original_id, importance, access_count, last_accessed, created_at, source, embedding, always_load, expires_at, tokens, ref_type, ref_target, is_lazy)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		content = excluded.content,
		summary = excluded.summary,
		importance = excluded.importance,
		last_accessed = excluded.last_accessed,
		source = excluded.source,
		embedding = excluded.embedding,
		always_load = excluded.always_load,
		expires_at = excluded.expires_at,
		tokens = excluded.tokens,
		ref_type = excluded.ref_type,
		ref_target = excluded.ref_target,
		is_lazy = excluded.is_lazy
	`
	_, err = tx.Exec(query,
		m.ID, m.Content, nullString(m.Summary), nullString(m.OriginalID),
		m.Importance, m.AccessCount,
		m.LastAccessed.Unix(), m.CreatedAt.Unix(), m.Source, embJSON,
		m.AlwaysLoad, nullInt64Ptr(m.ExpiresAt), m.Tokens,
		m.RefType, nullString(m.RefTarget), m.IsLazy,
	)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	// Replace tags (delete old, insert new)
	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = ?", m.ID); err != nil {
		return fmt.Errorf("delete old tags: %w", err)
	}
	if len(m.Tags) > 0 {
		tagStmt, err := tx.Prepare("INSERT INTO memory_tags (memory_id, tag) VALUES (?, ?)")
		if err != nil {
			return fmt.Errorf("prepare tag insert: %w", err)
		}
		defer tagStmt.Close()

		for _, tag := range m.Tags {
			if _, err := tagStmt.Exec(m.ID, tag); err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
		}
	}

	return tx.Commit()
}

// Get retrieves a memory by ID and updates access tracking.
func (s *Store) Get(id string) (*Memory, error) {
	// Support prefix matching (e.g., first 8 chars of UUID)
	query := `
	SELECT id, content, summary, original_id, importance, access_count, last_accessed, created_at, source, embedding, always_load, expires_at, tokens, ref_type, ref_target, is_lazy
	FROM memories
	WHERE id = ? OR id LIKE ?
	`
	row := s.db.QueryRow(query, id, id+"%")

	var m Memory
	var summary, originalID, refTarget sql.NullString
	var embJSON []byte
	var lastAccessed, createdAt int64
	var expiresAt sql.NullInt64

	err := row.Scan(
		&m.ID, &m.Content, &summary, &originalID,
		&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
		&m.AlwaysLoad, &expiresAt, &m.Tokens,
		&m.RefType, &refTarget, &m.IsLazy,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}

	m.Summary = summary.String
	m.OriginalID = originalID.String
	m.RefTarget = refTarget.String
	m.LastAccessed = time.Unix(lastAccessed, 0)
	m.CreatedAt = time.Unix(createdAt, 0)
	if expiresAt.Valid {
		exp := time.Unix(expiresAt.Int64, 0)
		m.ExpiresAt = &exp
	}
	
	// If this is a lazy-loaded reference, load content on-demand
	if m.IsLazy {
		if err := s.loadLazyContent(&m); err != nil {
			return nil, fmt.Errorf("load lazy content: %w", err)
		}
	}

	if err := json.Unmarshal(embJSON, &m.Embedding); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
	}

	// Fetch tags
	tagRows, err := s.db.Query("SELECT tag FROM memory_tags WHERE memory_id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer tagRows.Close()

	m.Tags = []string{}
	for tagRows.Next() {
		var tag string
		if err := tagRows.Scan(&tag); err != nil {
			continue
		}
		m.Tags = append(m.Tags, tag)
	}

	// Update access tracking synchronously
	s.updateAccess(id)
	
	// Update the returned struct to reflect the access tracking changes
	m.AccessCount++
	m.LastAccessed = time.Now()
	m.Importance = math.Min(1.0, m.Importance*1.01)

	return &m, nil
}

// updateAccess increments access count and updates last accessed time.
func (s *Store) updateAccess(id string) {
	query := `
	UPDATE memories
	SET access_count = access_count + 1,
	    last_accessed = ?,
	    importance = MIN(1.0, importance * 1.01)
	WHERE id = ?
	`
	s.db.Exec(query, time.Now().Unix(), id)
}