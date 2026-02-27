# Memory System

Inber provides **persistent memory** that survives across sessions, allowing agents to recall past conversations, decisions, and learnings.

## Architecture

```
Agent Tools          Memory Store           Search Engine
───────────         ──────────────         ──────────────

memory_search  ──►  SQLite Database  ──►  TF-IDF Embeddings
memory_save    ──►  ┌────────────┐   ──►  ┌──────────────┐
memory_expand  ──►  │ Memories   │   ──►  │ 256-dim      │
memory_forget  ──►  │ - Content  │   ──►  │ Bag-of-Words │
                    │ - Tags     │   ──►  │ Vectors      │
                    │ - Metadata │   ──►  └──────────────┘
                    └────────────┘
                         │
                         ▼
                    Ranking System
                    ──────────────
                    • Similarity
                    • Importance
                    • Recency
                    • Decay
```

## Core Concepts

### 1. Memory Structure

Each memory is a structured record with:

```go
type Memory struct {
    ID            string    // Unique identifier (UUID)
    Content       string    // The actual memory text
    Tags          []string  // Categories (e.g., "preference", "code")
    Importance    float64   // Score 0-1 (higher = more important)
    Source        string    // "user", "agent", or "system"
    CreatedAt     time.Time // When it was created
    LastAccessed  time.Time // Last retrieval time
    AccessCount   int       // How many times accessed
    DecayFactor   float64   // Importance decay rate
    Forgotten     bool      // Soft delete flag
    OriginalID    string    // For compacted memories (future)
}
```

### 2. Semantic Search

Memories are searchable using **TF-IDF embeddings**:

1. **Query** → Convert to 256-dimensional vector
2. **Compare** → Compute cosine similarity with all memory vectors
3. **Rank** → Combine similarity + importance + recency
4. **Return** → Top N matches

**Ranking formula:**
```
score = (similarity * 2.0) + importance + recency_boost
```

Where:
- `similarity`: Cosine similarity (0-1)
- `importance`: Memory importance score (0-1)
- `recency_boost`: 0.1 if accessed in last 7 days, else 0

### 3. Importance Scoring

Importance determines how likely a memory is to be recalled:

| Score | Category | Use Cases |
|-------|----------|-----------|
| 0.9-1.0 | Critical | Security keys, core requirements, fundamental constraints |
| 0.7-0.8 | High | User preferences, architectural decisions, key patterns |
| 0.5-0.6 | Medium | Useful context, coding patterns, common solutions |
| 0.3-0.4 | Low | Minor observations, temporary notes |
| 0.1-0.2 | Minimal | Historical trivia, outdated references |

### 4. Decay System

Memories naturally **decay** over time if not accessed:

- **DecayFactor**: 0.95 (default) means importance drops 5% per week
- **Access resets decay**: Retrieving a memory restores its importance
- **User memories decay slower**: Source="user" gets 0.98 decay factor

**Example:**
```
Week 0: Importance 0.8
Week 1: 0.8 * 0.95 = 0.76
Week 2: 0.76 * 0.95 = 0.72
[Access occurs - importance restored]
Week 3: 0.8 (reset)
```

## Agent Tools

### memory_search

Find relevant memories using semantic similarity.

**When to use:**
- Starting a new task in a familiar project
- User asks "remember when..."
- Need context about past decisions
- Looking for patterns from previous work

**Example:**
```json
{
  "query": "how did we handle authentication errors last time?",
  "limit": 5
}
```

**Output:**
```
1. [550e8400-...] (importance: 0.85, accessed: 3 times, source: agent)
   Tags: auth, errors, debugging
   We resolved authentication errors by adding retry logic with exponential
   backoff. User wanted 3 retries max with 1s initial delay.

2. [661f9511-...] (importance: 0.75, accessed: 1 time, source: user)
   Tags: auth, preference
   User prefers OAuth2 over basic auth for all external APIs.
```

---

### memory_save

Store important information for future recall.

**When to save:**
- ✅ User states a preference or requirement
- ✅ You learn something important about the codebase
- ✅ A bug fix that might recur
- ✅ Architecture decisions or constraints
- ✅ Successful patterns or solutions
- ❌ Temporary command outputs
- ❌ Generic knowledge (already in training data)
- ❌ Obvious facts

**Example:**
```json
{
  "content": "User wants all API responses to include request_id for debugging. This is a hard requirement for production.",
  "tags": ["api", "requirement", "debugging"],
  "importance": 0.9,
  "source": "user"
}
```

---

### memory_expand

Get full details of a specific memory.

**When to use:**
- Search results are truncated
- Need to see memory metadata (created date, access count)
- Reviewing memory lineage (for compacted memories)

**Example:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

### memory_forget

Mark a memory as irrelevant (soft delete).

**When to use:**
- Information is outdated (e.g., old API endpoints)
- Incorrect information was saved
- Temporary notes no longer needed
- User requests deletion

**Note:** Forgotten memories remain in the database but don't appear in searches.

---

## Auto-Loading into Context

At session start, high-importance memories (score > 0.7) are automatically loaded into context:

1. Search for recent + important memories
2. Convert to context chunks
3. Tag with original memory tags
4. Priority = importance score

This ensures critical information is always available without explicit search.

## Storage

### Database Location
```
.inber/memory.db
```

### Schema
```sql
CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    tags TEXT,  -- JSON array
    importance REAL DEFAULT 0.5,
    source TEXT DEFAULT 'agent',
    created_at INTEGER,
    last_accessed INTEGER,
    access_count INTEGER DEFAULT 0,
    decay_factor REAL DEFAULT 0.95,
    forgotten BOOLEAN DEFAULT 0,
    original_id TEXT,
    embedding BLOB  -- TF-IDF vector
);
```

### Indexes
- `idx_importance`: Fast filtering by importance
- `idx_forgotten`: Exclude forgotten memories
- `idx_tags`: Tag-based filtering (JSON)

## Embedding System

### Current: TF-IDF (Bag-of-Words)

**Advantages:**
- Fast (no API calls)
- Deterministic
- No external dependencies
- Works offline

**Limitations:**
- Word order ignored
- Synonyms not detected
- Less semantic understanding than neural embeddings

**Vector format:**
- Dimensions: 256
- Encoding: Term frequency × inverse document frequency
- Similarity: Cosine similarity

### Future: Neural Embeddings

Planned upgrade to models like:
- OpenAI `text-embedding-3-small`
- Local sentence transformers
- Custom fine-tuned embeddings

## Best Practices

### For Agents

1. **Be selective**: Don't save everything, focus on high-value information
2. **Use descriptive tags**: `["auth", "bug-fix", "oauth"]` not just `["code"]`
3. **Set importance thoughtfully**: Most memories should be 0.5-0.7
4. **Search before asking user**: Check if you already know the answer
5. **Forget outdated info**: Clean up obsolete memories proactively

### For Users

1. **State preferences clearly**: "I prefer X" triggers memory_save
2. **Correct mistakes**: If agent saves wrong info, ask to forget it
3. **Review memories**: Periodically ask "what do you remember about X?"

### Tag Conventions

**Common tags:**
- `preference` - User preferences
- `requirement` - Hard requirements
- `bug-fix` - Solutions to bugs
- `pattern` - Reusable patterns
- `decision` - Architectural decisions
- `constraint` - Technical constraints
- `fact` - Factual information about the project

**Domain-specific:**
- `auth`, `api`, `database`, `frontend`, `backend`
- `golang`, `python`, `javascript`
- `debugging`, `performance`, `security`

## Memory Lifecycle

```
1. Creation
   ↓
2. Storage (SQLite + embedding)
   ↓
3. Retrieval (search or auto-load)
   ↓
4. Access (resets decay)
   ↓
5. Decay (weekly, if not accessed)
   ↓
6. Compaction (future: summarize old memories)
   ↓
7. Forgetting (soft delete)
```

## Future Enhancements

### Compaction
- Summarize old memories into higher-level insights
- Keep lineage pointers for memory_expand
- Reduce storage size while preserving knowledge

### Reflection
- Auto-generate insights from recent memories
- "What patterns do I see in my recent work?"
- Generative Agents-style reflection

### Cross-Session Awareness
- "I'm working on the same problem I saw last week"
- Automatic context loading based on task similarity

### Memory Clustering
- Group related memories
- "Show me all memories about authentication"

### Collaborative Memory
- Share memories between agents
- Access control and permissions
