// Package memory provides persistent, searchable memory across sessions.
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		summary TEXT,
		original_id TEXT,
		tags TEXT NOT NULL,
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
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{
		db:       db,
		embedder: NewEmbedder(),
	}, nil
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

	// Serialize tags and embedding
	tagsJSON, err := json.Marshal(m.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	embJSON, err := json.Marshal(m.Embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	query := `
	INSERT INTO memories (id, content, summary, original_id, tags, importance, access_count, last_accessed, created_at, source, embedding)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = s.db.Exec(query,
		m.ID, m.Content, nullString(m.Summary), nullString(m.OriginalID),
		string(tagsJSON), m.Importance, m.AccessCount,
		m.LastAccessed.Unix(), m.CreatedAt.Unix(), m.Source, embJSON,
	)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

// Get retrieves a memory by ID and updates access tracking.
func (s *Store) Get(id string) (*Memory, error) {
	query := `
	SELECT id, content, summary, original_id, tags, importance, access_count, last_accessed, created_at, source, embedding
	FROM memories
	WHERE id = ?
	`
	row := s.db.QueryRow(query, id)

	var m Memory
	var summary, originalID sql.NullString
	var tagsJSON, embJSON []byte
	var lastAccessed, createdAt int64

	err := row.Scan(
		&m.ID, &m.Content, &summary, &originalID, &tagsJSON,
		&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}

	m.Summary = summary.String
	m.OriginalID = originalID.String
	m.LastAccessed = time.Unix(lastAccessed, 0)
	m.CreatedAt = time.Unix(createdAt, 0)

	if err := json.Unmarshal(tagsJSON, &m.Tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	if err := json.Unmarshal(embJSON, &m.Embedding); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
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

	// Fetch all non-forgotten memories
	sqlQuery := `
	SELECT id, content, summary, original_id, tags, importance, access_count, last_accessed, created_at, source, embedding
	FROM memories
	WHERE importance > 0
	`
	rows, err := s.db.Query(sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	type scored struct {
		memory Memory
		score  float64
	}
	var candidates []scored

	for rows.Next() {
		var m Memory
		var summary, originalID sql.NullString
		var tagsJSON, embJSON []byte
		var lastAccessed, createdAt int64

		err := rows.Scan(
			&m.ID, &m.Content, &summary, &originalID, &tagsJSON,
			&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
		)
		if err != nil {
			continue
		}

		m.Summary = summary.String
		m.OriginalID = originalID.String
		m.LastAccessed = time.Unix(lastAccessed, 0)
		m.CreatedAt = time.Unix(createdAt, 0)

		if err := json.Unmarshal(tagsJSON, &m.Tags); err != nil {
			continue
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
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
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
	query := `UPDATE memories SET importance = 0 WHERE id = ?`
	res, err := s.db.Exec(query, id)
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
	query := `
	SELECT id, content, summary, original_id, tags, importance, access_count, last_accessed, created_at, source, embedding
	FROM memories
	WHERE importance >= ?
	ORDER BY created_at DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, minImportance, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent: %w", err)
	}
	defer rows.Close()

	var result []Memory
	for rows.Next() {
		var m Memory
		var summary, originalID sql.NullString
		var tagsJSON, embJSON []byte
		var lastAccessed, createdAt int64

		err := rows.Scan(
			&m.ID, &m.Content, &summary, &originalID, &tagsJSON,
			&m.Importance, &m.AccessCount, &lastAccessed, &createdAt, &m.Source, &embJSON,
		)
		if err != nil {
			continue
		}

		m.Summary = summary.String
		m.OriginalID = originalID.String
		m.LastAccessed = time.Unix(lastAccessed, 0)
		m.CreatedAt = time.Unix(createdAt, 0)

		if err := json.Unmarshal(tagsJSON, &m.Tags); err != nil {
			continue
		}
		if err := json.Unmarshal(embJSON, &m.Embedding); err != nil {
			continue
		}

		result = append(result, m)
	}

	return result, rows.Err()
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

// nullString returns a sql.NullString from a string.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
