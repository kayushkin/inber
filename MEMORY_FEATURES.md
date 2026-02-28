# Memory System Features

This document describes the two new memory system features implemented for keeping context lean without losing information.

## 1. Post-Turn Background Memory Extraction

After each turn completes and the response is delivered to the user, the system automatically extracts important facts, decisions, and preferences from the exchange and saves them to memory.

### How it works:

1. **Async Processing**: Runs in a background goroutine after the user receives their response (no blocking)
2. **Smart Filtering**: Only processes exchanges with >200 tokens (skips trivial "hi" → "hey" exchanges)
3. **LLM-based Extraction**: Uses a cheap model call with a focused prompt to identify important memories
4. **Duplicate Detection**: Checks for similar existing memories before saving (using Jaccard similarity)
5. **Structured Output**: Each extracted memory includes:
   - Content (concise 1-2 sentence fact/decision)
   - Importance score (0.0-1.0)
   - Tags (relevant keywords)

### Configuration:

```go
type ExtractionConfig struct {
    Enabled             bool    // Default: true
    MinExchangeTokens   int     // Default: 200
    Model               string  // Default: uses agent's model
    DuplicateThreshold  float64 // Default: 0.7 (Jaccard similarity)
    MaxSearchResults    int     // Default: 5
    MinImportance       float64 // Default: 0.3
}
```

### Example:

**User**: "I prefer using Go for backend services because of its simplicity"

**Assistant**: "Got it! I'll keep that in mind. Go is a great choice for backend work."

**Extracted memory** (saved in background):
```json
{
  "content": "User prefers using Go for backend services, values simplicity",
  "importance": 0.6,
  "tags": ["preference", "golang", "backend"]
}
```

## 2. Large Message Summarize-and-Stash

When user messages or assistant responses are very large (>1000 tokens), the system automatically:
1. Identifies the bulk content type (error dump, code block, log output, etc.)
2. Saves the full content to memory with appropriate tags
3. Replaces the large content in the conversation with a compact summary + memory reference

### How it works:

**For user messages** (before sending to LLM):
- Scans the message for large blocks (>1000 tokens)
- Detects content type using heuristics (no LLM needed):
  - Error dumps: contains "Error:", "Exception:", stack traces
  - Code blocks: multiple code fences (```)
  - Log output: timestamps + repeated patterns
  - File contents: file paths + line numbers
  - Default: "large-text"
- Saves full content to memory with tags like `["large-input", "error-dump", session_id]`
- Replaces the block with: `[Large content stashed — error dump, ~2400 tokens. Use memory_expand(id="abc123") to recall]`

**For assistant responses** (after receiving from LLM):
- User gets the full response (no modification)
- In conversation history for subsequent turns, large responses are stashed
- Prevents context bloat while keeping information accessible

### Configuration:

```go
type StashConfig struct {
    Enabled              bool    // Default: true
    UserMessageThreshold int     // Default: 1000 tokens
    AssistantThreshold   int     // Default: 1500 tokens
    MinBlockSize         int     // Default: 1000 tokens
    DefaultImportance    float64 // Default: 0.6
}
```

### Example:

**User**: *pastes a 3000-line error dump*

**System** (preprocessing):
1. Detects content type: "error-dump" (contains "panic:", stack traces)
2. Saves full dump to memory with ID `abc12345`
3. Sends to LLM: `[Large content stashed — error dump, ~12000 tokens. Use memory_expand(id="abc12345") to recall]`

**LLM** (sees summary):
- Can reason about whether it needs the full dump
- Can call `memory_expand(id="abc12345")` if needed
- No need to evaluate token cost — system handles that automatically

### Content Type Detection (No LLM Needed):

| Content Type | Detection Heuristic |
|--------------|---------------------|
| error-dump | Contains "Error:", "Exception:", "panic:", stack traces |
| code-block | Multiple code fences (```), lots of indentation |
| log-output | Timestamps + repeated patterns + log levels (INFO, WARN) |
| file-contents | File paths + line numbers (`:123:`) |
| large-text | Default fallback |

## Key Benefits:

1. **No blocking**: User gets their response immediately, extraction happens in background
2. **Cost-efficient**: Only processes substantive exchanges, keeps extraction prompts <500 tokens
3. **Smart economics**: The LLM doesn't have to decide "is it worth the tokens?" — the system handles it
4. **Seamless recall**: Agent can use `memory_search` or `memory_expand` to pull back stashed content
5. **Automatic tagging**: Content type, session ID, source type all tagged automatically

## Integration:

Both features are integrated into `Engine.RunTurn()`:

```go
// 1. BEFORE sending to LLM: stash large user messages
if tokens > cfg.UserMessageThreshold {
    modifiedInput, stashed, _ := DetectAndStashLargeBlocks(input, sessionID, memStore, cfg)
    // ... use modifiedInput for LLM request
}

// 2. AFTER receiving response: background extraction
go BackgroundExtractMemories(ctx, client, userMessage, assistantResponse, ...)

// 3. BEFORE next turn: stash large assistant responses in history
if responseTokens > cfg.AssistantThreshold {
    // ... stash and replace in conversation history
}
```

## Testing:

Run the test suite to verify both features:

```bash
cd /home/slava/life/repos/inber
go test ./cmd/inber/ -run "TestDetectContentType|TestStashLargeContent|TestDetectAndStashLargeBlocks|TestCalculateSimilarity" -v
```

All tests should pass:
- Content type detection for various inputs
- Large content stashing and retrieval
- Block detection and replacement
- Similarity calculation for duplicate detection

## Future Enhancements:

1. **Configurable per-agent**: Different thresholds for orchestrator vs. coder agents
2. **Adaptive thresholds**: Learn optimal thresholds based on usage patterns
3. **Semantic embeddings**: Replace Jaccard similarity with real embeddings for duplicate detection
4. **Extraction templates**: Role-specific extraction prompts (orchestrator vs. coder)
5. **Memory consolidation**: Periodically merge similar memories to prevent duplication
