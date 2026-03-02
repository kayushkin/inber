// Package memory provides persistent, searchable memory across sessions.
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	db, err := sql.Open("sqlite3", dbPath)
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

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT,
		model TEXT,
		started_at INTEGER NOT NULL,
		ended_at INTEGER,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		cost REAL DEFAULT 0,
		summary TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_agent_name ON sessions(agent_name);

	CREATE TABLE IF NOT EXISTS session_tags (
		session_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_session_tags_session_id ON session_tags(session_id);
	CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);

	CREATE TABLE IF NOT EXISTS memory_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		turn_number INTEGER NOT NULL,
		usage_type TEXT NOT NULL,
		accessed_at INTEGER NOT NULL,
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_memory_usage_memory_id ON memory_usage(memory_id);
	CREATE INDEX IF NOT EXISTS idx_memory_usage_session_id ON memory_usage(session_id);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
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

// Search finds memories matching the query, ranked by similarity, recency, and importance.
func (s *Store) Search(query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	// Generate query embedding
	queryEmb := s.embedder.Embed(query)

	// Fetch all non-forgotten, non-expired memories
	now := time.Now()
	sqlQuery := `
	SELECT id, content, summary, original_id, importance, access_count, last_accessed, created_at, source, embedding, always_load, expires_at, tokens, ref_type, ref_target, is_lazy
	FROM memories
	WHERE importance > 0
	  AND (expires_at IS NULL OR expires_at > ?)
	`
	rows, err := s.db.Query(sqlQuery, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()

	type scored struct {
		memory Memory
		score  float64
	}
	var candidates []scored
	memoryIDs := []string{}

	for rows.Next() {
		var m Memory
		var summary, originalID, refTarget sql.NullString
		var embJSON []byte
		var lastAccessed, createdAt int64
		var expiresAt sql.NullInt64

		err := rows.Scan(
			&m.ID, &m.Content, &summary, &originalID,
			&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
			&m.AlwaysLoad, &expiresAt, &m.Tokens,
			&m.RefType, &refTarget, &m.IsLazy,
		)
		if err != nil {
			continue
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

		if err := json.Unmarshal(embJSON, &m.Embedding); err != nil {
			continue
		}

		// Calculate similarity
		similarity := cosineSimilarity(queryEmb, m.Embedding)

		// Calculate recency boost (decays over time)
		daysSinceAccess := now.Sub(m.LastAccessed).Hours() / 24
		recencyBoost := math.Pow(0.99, daysSinceAccess)

		// Combined score
		score := similarity * m.Importance * recencyBoost

		candidates = append(candidates, scored{memory: m, score: score})
		memoryIDs = append(memoryIDs, m.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Fetch tags for all memories in a single query
	if len(memoryIDs) > 0 {
		tagMap := make(map[string][]string)
		
		// Build IN clause
		placeholders := make([]string, len(memoryIDs))
		args := make([]interface{}, len(memoryIDs))
		for i, id := range memoryIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		
		tagQuery := fmt.Sprintf("SELECT memory_id, tag FROM memory_tags WHERE memory_id IN (%s)", 
			strings.Join(placeholders, ","))
		
		tagRows, err := s.db.Query(tagQuery, args...)
		if err == nil {
			defer tagRows.Close()
			for tagRows.Next() {
				var memID, tag string
				if err := tagRows.Scan(&memID, &tag); err == nil {
					tagMap[memID] = append(tagMap[memID], tag)
				}
			}
		}
		
		// Attach tags to memories
		for i := range candidates {
			candidates[i].memory.Tags = tagMap[candidates[i].memory.ID]
			if candidates[i].memory.Tags == nil {
				candidates[i].memory.Tags = []string{}
			}
		}
	}

	// Sort by score (descending)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Take top N
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := make([]Memory, len(candidates))
	for i, c := range candidates {
		result[i] = c.memory
	}

	return result, nil
}

// Forget marks a memory as forgotten (soft delete by setting importance to 0).
func (s *Store) Forget(id string) error {
	query := `UPDATE memories SET importance = 0 WHERE id = ? OR id LIKE ?`
	res, err := s.db.Exec(query, id, id+"%")
	if err != nil {
		return fmt.Errorf("forget memory: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory not found: %s", id)
	}
	return nil
}

// DecayImportance applies time-based decay to all memories.
// Called periodically (e.g., daily) to gradually reduce importance of unused memories.
func (s *Store) DecayImportance() error {
	now := time.Now()
	query := `
	UPDATE memories
	SET importance = importance * ?
	WHERE last_accessed < ?
	`
	dayAgo := now.Add(-24 * time.Hour).Unix()
	_, err := s.db.Exec(query, 0.99, dayAgo)
	return err
}

// ListRecent returns the N most recently created memories with importance > threshold.
func (s *Store) ListRecent(limit int, minImportance float64) ([]Memory, error) {
	now := time.Now()
	query := `
	SELECT id, content, summary, original_id, importance, access_count, last_accessed, created_at, source, embedding, always_load, expires_at, tokens, ref_type, ref_target, is_lazy
	FROM memories
	WHERE importance >= ?
	  AND (expires_at IS NULL OR expires_at > ?)
	ORDER BY created_at DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, minImportance, now.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("query recent: %w", err)
	}
	defer rows.Close()

	var result []Memory
	var memoryIDs []string
	
	for rows.Next() {
		var m Memory
		var summary, originalID, refTarget sql.NullString
		var embJSON []byte
		var lastAccessed, createdAt int64
		var expiresAt sql.NullInt64

		err := rows.Scan(
			&m.ID, &m.Content, &summary, &originalID,
			&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
			&m.AlwaysLoad, &expiresAt, &m.Tokens,
			&m.RefType, &refTarget, &m.IsLazy,
		)
		if err != nil {
			continue
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

		if err := json.Unmarshal(embJSON, &m.Embedding); err != nil {
			continue
		}

		result = append(result, m)
		memoryIDs = append(memoryIDs, m.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch tags for all memories
	if len(memoryIDs) > 0 {
		tagMap := make(map[string][]string)
		
		placeholders := make([]string, len(memoryIDs))
		args := make([]interface{}, len(memoryIDs))
		for i, id := range memoryIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		
		tagQuery := fmt.Sprintf("SELECT memory_id, tag FROM memory_tags WHERE memory_id IN (%s)", 
			strings.Join(placeholders, ","))
		
		tagRows, err := s.db.Query(tagQuery, args...)
		if err == nil {
			defer tagRows.Close()
			for tagRows.Next() {
				var memID, tag string
				if err := tagRows.Scan(&memID, &tag); err == nil {
					tagMap[memID] = append(tagMap[memID], tag)
				}
			}
		}
		
		// Attach tags to memories
		for i := range result {
			result[i].Tags = tagMap[result[i].ID]
			if result[i].Tags == nil {
				result[i].Tags = []string{}
			}
		}
	}

	return result, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CompactionResult describes what was compacted.
type CompactionResult struct {
	OriginalIDs []string
	NewID       string
	Tags        []string
	Count       int
}

// Compact finds old, low-access, low-importance memories, groups them by tags,
// creates compacted summaries, and soft-deletes originals.
func (s *Store) Compact(minAge time.Duration, minCount int) ([]CompactionResult, error) {
	cutoff := time.Now().Add(-minAge).Unix()

	query := `
	SELECT id, content, importance, access_count, created_at
	FROM memories
	WHERE created_at < ? AND access_count < ? AND importance < 0.7 AND importance > 0
	`
	rows, err := s.db.Query(query, cutoff, minCount)
	if err != nil {
		return nil, fmt.Errorf("query compactable: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id      string
		content string
	}
	var candidates []candidate
	var candidateIDs []string

	for rows.Next() {
		var id, content string
		var importance float64
		var accessCount int
		var createdAt int64
		if err := rows.Scan(&id, &content, &importance, &accessCount, &createdAt); err != nil {
			continue
		}
		candidates = append(candidates, candidate{id: id, content: content})
		candidateIDs = append(candidateIDs, id)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Fetch tags for candidates
	tagMap := make(map[string][]string)
	if len(candidateIDs) > 0 {
		placeholders := make([]string, len(candidateIDs))
		args := make([]interface{}, len(candidateIDs))
		for i, id := range candidateIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		tagQuery := fmt.Sprintf("SELECT memory_id, tag FROM memory_tags WHERE memory_id IN (%s)",
			strings.Join(placeholders, ","))
		tagRows, err := s.db.Query(tagQuery, args...)
		if err == nil {
			defer tagRows.Close()
			for tagRows.Next() {
				var memID, tag string
				if err := tagRows.Scan(&memID, &tag); err == nil {
					tagMap[memID] = append(tagMap[memID], tag)
				}
			}
		}
	}

	// Group by primary tag (first tag, or "untagged")
	groups := make(map[string][]candidate)
	groupTags := make(map[string]map[string]bool)
	for _, c := range candidates {
		tags := tagMap[c.id]
		key := "untagged"
		if len(tags) > 0 {
			key = tags[0]
		}
		groups[key] = append(groups[key], c)
		if groupTags[key] == nil {
			groupTags[key] = make(map[string]bool)
		}
		for _, t := range tags {
			groupTags[key][t] = true
		}
	}

	var results []CompactionResult

	for groupKey, members := range groups {
		if len(members) < 2 {
			continue // don't compact single memories
		}

		// Build combined content
		var parts []string
		var origIDs []string
		for _, m := range members {
			parts = append(parts, m.content)
			origIDs = append(origIDs, m.id)
		}
		combined := strings.Join(parts, "\n---\n")
		if len(combined) > 2000 {
			combined = combined[:2000] + "..."
		}

		// Collect all tags for the group
		var allTags []string
		for t := range groupTags[groupKey] {
			allTags = append(allTags, t)
		}

		newID := fmt.Sprintf("compact-%s-%d", groupKey, time.Now().UnixNano())
		newMem := Memory{
			ID:         newID,
			Content:    fmt.Sprintf("[Compacted from %d memories]\n%s", len(members), combined),
			Tags:       allTags,
			Importance: 0.5,
			Source:     "compaction",
			OriginalID: origIDs[0],
		}

		if err := s.Save(newMem); err != nil {
			continue
		}

		// Soft-delete originals
		for _, id := range origIDs {
			s.db.Exec("UPDATE memories SET importance = 0 WHERE id = ?", id)
		}

		results = append(results, CompactionResult{
			OriginalIDs: origIDs,
			NewID:       newID,
			Tags:        allTags,
			Count:       len(members),
		})
	}

	return results, nil
}

// nullString returns a sql.NullString from a string.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// Session represents a tracked agent session.
type Session struct {
	ID           string
	AgentName    string
	Model        string
	StartedAt    time.Time
	EndedAt      time.Time
	InputTokens  int
	OutputTokens int
	Cost         float64
	Summary      string
	Tags         []string
}

// SaveSession stores a session record.
func (s *Store) SaveSession(sess Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT INTO sessions (id, agent_name, model, started_at, ended_at, input_tokens, output_tokens, cost, summary)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query,
		sess.ID, sess.AgentName, sess.Model,
		sess.StartedAt.Unix(), nullInt64(sess.EndedAt),
		sess.InputTokens, sess.OutputTokens, sess.Cost, nullString(sess.Summary),
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	// Insert tags
	if len(sess.Tags) > 0 {
		tagStmt, err := tx.Prepare("INSERT INTO session_tags (session_id, tag) VALUES (?, ?)")
		if err != nil {
			return fmt.Errorf("prepare tag insert: %w", err)
		}
		defer tagStmt.Close()

		for _, tag := range sess.Tags {
			if _, err := tagStmt.Exec(sess.ID, tag); err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
		}
	}

	return tx.Commit()
}

// TrackMemoryUsage records when a memory was used in a session.
func (s *Store) TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error {
	query := `
	INSERT INTO memory_usage (memory_id, session_id, turn_number, usage_type, accessed_at)
	VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, memoryID, sessionID, turnNumber, usageType, time.Now().Unix())
	return err
}

// nullInt64 returns a sql.NullInt64 from a time.Time.
func nullInt64(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: t.Unix(), Valid: true}
}

func nullInt64Ptr(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: t.Unix(), Valid: true}
}
