// Package memory — remote.go provides a MemoryStore backed by agent-store.
//
// This adapter imports github.com/kayushkin/agent-store directly (in-process).
// When agent-store gains an HTTP API + Go client, we can swap the underlying
// implementation without changing inber's MemoryStore interface.
//
// Higher-level operations that agent-store doesn't support yet (Search with
// ranking, BuildContext, PrepareSession, sessions, tool registry) are handled
// by embedding a local SQLite store for now. As agent-store gains these
// capabilities, we'll migrate them one by one.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	agentstore "github.com/kayushkin/agent-store"
)

// RemoteStore delegates memory persistence to agent-store while keeping
// local logic for search ranking, context building, and session management.
//
// Architecture:
//   - Core CRUD (Save/Get/Forget) → agent-store
//   - Search, BuildContext, PrepareSession → local (reads from agent-store)
//   - Sessions, ToolRegistry → local SQLite (until agent-store supports them)
type RemoteStore struct {
	as        *agentstore.Store
	agentSlug string // scopes all operations to this agent
	agentID   int64  // resolved agent ID in agent-store

	// Local SQLite for operations agent-store doesn't support yet.
	// This is the same Store type — we delegate to it for Search, BuildContext, etc.
	// Over time, these will migrate to agent-store.
	local *Store
}

// NewRemoteStore creates a RemoteStore backed by an agent-store database.
// agentStoreDBPath is the path to agent-store's SQLite DB (e.g. ~/.config/agent-store/agents.db).
// localDBPath is the path for inber's local SQLite (for search, sessions, etc.).
// agentSlug identifies this agent in agent-store.
func NewRemoteStore(agentStoreDBPath, localDBPath, agentSlug string) (*RemoteStore, error) {
	as, err := agentstore.Open(agentStoreDBPath)
	if err != nil {
		return nil, fmt.Errorf("open agent-store: %w", err)
	}

	// Resolve agent ID (create if needed)
	agent, err := as.GetAgentBySlug(agentSlug)
	if err != nil {
		// Agent doesn't exist yet — create it
		agent = &agentstore.Agent{
			Slug:        agentSlug,
			DisplayName: agentSlug,
			Enabled:     true,
		}
		if err := as.UpsertAgent(agent); err != nil {
			as.Close()
			return nil, fmt.Errorf("create agent in agent-store: %w", err)
		}
	}

	// Open local store for operations not yet in agent-store
	local, err := NewStore(localDBPath)
	if err != nil {
		as.Close()
		return nil, fmt.Errorf("open local store: %w", err)
	}

	return &RemoteStore{
		as:        as,
		agentSlug: agentSlug,
		agentID:   agent.ID,
		local:     local,
	}, nil
}

// Close closes both agent-store and local connections.
func (r *RemoteStore) Close() error {
	var errs []error
	if err := r.as.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close agent-store: %w", err))
	}
	if err := r.local.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close local: %w", err))
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Save persists a memory to both agent-store and local store.
// Agent-store is the source of truth; local is kept in sync for search.
func (r *RemoteStore) Save(m Memory) error {
	// Save to agent-store
	asm := toAgentStoreMemory(m, r.agentID)
	if err := r.as.SaveMemory(asm); err != nil {
		return fmt.Errorf("agent-store save: %w", err)
	}

	// Mirror to local store for search/BuildContext
	return r.local.Save(m)
}

// Get retrieves a memory by ID from agent-store, falling back to local.
func (r *RemoteStore) Get(id string) (*Memory, error) {
	// Try agent-store first
	asm, err := r.as.GetMemory(id)
	if err == nil {
		m := fromAgentStoreMemory(asm)
		// Update access tracking locally
		r.local.updateAccess(id)
		return m, nil
	}

	// Fall back to local (for memories not yet migrated, or prefix-match)
	return r.local.Get(id)
}

// Search delegates to local store (agent-store doesn't have ranked search yet).
func (r *RemoteStore) Search(query string, limit int) ([]Memory, error) {
	return r.local.Search(query, limit)
}

// Forget soft-deletes in both stores.
func (r *RemoteStore) Forget(id string) error {
	// In agent-store, delete the memory
	if err := r.as.DeleteMemory(id); err != nil {
		// Log but don't fail — it may not exist in agent-store
		fmt.Fprintf(os.Stderr, "warning: agent-store forget(%s): %v\n", id, err)
	}
	// In local store, soft-delete (importance=0)
	return r.local.Forget(id)
}

// DecayImportance delegates to local store.
func (r *RemoteStore) DecayImportance() error {
	return r.local.DecayImportance()
}

// ListRecent delegates to local store.
func (r *RemoteStore) ListRecent(limit int, minImportance float64) ([]Memory, error) {
	return r.local.ListRecent(limit, minImportance)
}

// Compact delegates to local store.
func (r *RemoteStore) Compact(minAge time.Duration, minCount int) ([]CompactionResult, error) {
	return r.local.Compact(minAge, minCount)
}

// BuildContext delegates to local store.
func (r *RemoteStore) BuildContext(req BuildContextRequest) ([]Memory, int, error) {
	return r.local.BuildContext(req)
}

// PrepareSession delegates to local store.
func (r *RemoteStore) PrepareSession(cfg PrepareSessionConfig) error {
	return r.local.PrepareSession(cfg)
}

// LoadToolRegistry delegates to local store.
func (r *RemoteStore) LoadToolRegistry(tools []ToolMetadata) error {
	return r.local.LoadToolRegistry(tools)
}

// UpdateToolUsageSummary delegates to local store.
func (r *RemoteStore) UpdateToolUsageSummary(toolName, summary string, ttlSeconds int64) error {
	return r.local.UpdateToolUsageSummary(toolName, summary, ttlSeconds)
}

// SaveSession delegates to local store.
func (r *RemoteStore) SaveSession(sess Session) error {
	return r.local.SaveSession(sess)
}

// TrackMemoryUsage delegates to local store.
func (r *RemoteStore) TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error {
	return r.local.TrackMemoryUsage(memoryID, sessionID, turnNumber, usageType)
}

// --- Conversion helpers ---

func toAgentStoreMemory(m Memory, agentID int64) *agentstore.Memory {
	asm := &agentstore.Memory{
		ID:          m.ID,
		Content:     m.Content,
		Summary:     m.Summary,
		OriginalID:  m.OriginalID,
		Importance:  m.Importance,
		AccessCount: m.AccessCount,
		Source:      m.Source,
		AgentID:     &agentID,
		Tags:        m.Tags,
		Tokens:      m.Tokens,
	}

	// Map inber's RefType to agent-store's Kind
	switch m.RefType {
	case "identity":
		asm.Kind = "identity"
	case "file":
		asm.Kind = "file"
	case "tools":
		asm.Kind = "tools"
	default:
		asm.Kind = "memory"
	}

	// Map scope from source
	switch m.Source {
	case "system":
		asm.Scope = "system"
	case "user":
		asm.Scope = "user"
	default:
		asm.Scope = "agent"
	}

	if !m.LastAccessed.IsZero() {
		ts := m.LastAccessed.Unix()
		asm.LastAccessed = &ts
	}
	if !m.CreatedAt.IsZero() {
		asm.CreatedAt = m.CreatedAt.Unix()
	}
	if m.ExpiresAt != nil {
		ts := m.ExpiresAt.Unix()
		asm.ExpiresAt = &ts
	}

	// Store embedding as JSON in Summary field if agent-store doesn't have embedding column
	// TODO: agent-store should add an embedding column
	if len(m.Embedding) > 0 {
		if embJSON, err := json.Marshal(m.Embedding); err == nil {
			_ = embJSON // Don't stuff embedding into summary — wait for agent-store to add embedding support
		}
	}

	return asm
}

func fromAgentStoreMemory(asm *agentstore.Memory) *Memory {
	m := &Memory{
		ID:          asm.ID,
		Content:     asm.Content,
		Summary:     asm.Summary,
		OriginalID:  asm.OriginalID,
		Importance:  asm.Importance,
		AccessCount: asm.AccessCount,
		Source:      asm.Source,
		Tags:        asm.Tags,
		Tokens:      asm.Tokens,
	}

	if m.Tags == nil {
		m.Tags = []string{}
	}

	// Map kind back to RefType
	switch asm.Kind {
	case "identity":
		m.RefType = "identity"
	case "file":
		m.RefType = "file"
	case "tools":
		m.RefType = "tools"
	default:
		m.RefType = "memory"
	}

	// Map scope back to source if source is empty
	if m.Source == "" {
		switch asm.Scope {
		case "system":
			m.Source = "system"
		case "user":
			m.Source = "user"
		default:
			m.Source = "agent"
		}
	}

	if asm.LastAccessed != nil {
		m.LastAccessed = time.Unix(*asm.LastAccessed, 0)
	}
	if asm.CreatedAt != 0 {
		m.CreatedAt = time.Unix(asm.CreatedAt, 0)
	}
	if asm.ExpiresAt != nil {
		exp := time.Unix(*asm.ExpiresAt, 0)
		m.ExpiresAt = &exp
	}

	// Infer AlwaysLoad from kind
	if asm.Kind == "identity" || asm.Kind == "tools" {
		m.AlwaysLoad = true
	}

	return m
}

// Compile-time interface check
var _ MemoryStore = (*RemoteStore)(nil)
