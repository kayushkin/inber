# Context Management Logging

## Overview

The session logger now records all context management operations (summarization, pruning, stashing, and compaction) to the session JSONL file. This provides full visibility into how the conversation is being optimized over time.

## Log Entry Types

### 1. Summarization (`role: "summarize"`)

Logged when conversation history is compressed into a summary.

```json
{
  "ts": "2025-03-01T10:30:45Z",
  "role": "summarize",
  "content": "summarized 15 turns → 250 token summary (kept 10 recent messages, memory: mem_abc123)",
  "request": {
    "summarized_turns": 15,
    "summary_tokens": 250,
    "kept_messages": 10,
    "memory_id": "mem_abc123"
  }
}
```

**When it happens:**
- Conversation reaches turn threshold (default: 70 for default role, 40 for coding agents)
- Or token count exceeds threshold (default: 50K tokens)

**What it means:**
- Old turns are condensed into a summary block
- Full conversation saved to memory store for retrieval
- Summary block + recent messages kept in prompt

### 2. Pruning (`role: "prune"`)

Logged when conversation messages are truncated or removed.

```json
{
  "ts": "2025-03-01T10:31:12Z",
  "role": "prune",
  "content": "pruned 5 messages (1200 tokens freed, strategy: role-based-truncation)",
  "request": {
    "removed": 5,
    "tokens_freed": 1200,
    "strategy": "role-based-truncation"
  }
}
```

**When it happens:**
- After summarization (prune what remains)
- When conversation exceeds token budget

**Strategies:**
- `role-based-truncation`: Different retention windows per message role
- `age-based-removal`: Remove messages older than N turns
- `importance-scoring`: Remove low-importance messages

**What it means:**
- Tool results truncated (old ones summarized, very old ones dropped)
- Assistant messages truncated (old turns condensed to 2-3 sentences)
- User messages never truncated

### 3. Stashing (`role: "stash"`)

Logged when large content blocks are moved to memory store and replaced with references.

```json
{
  "ts": "2025-03-01T10:29:30Z",
  "role": "stash",
  "content": "stashed 2 large blocks from user message (800 tokens)",
  "request": {
    "message_type": "user",
    "block_count": 2,
    "tokens": 800
  }
}
```

**When it happens:**
- User message > 1000 tokens (default threshold)
- Assistant response > 1500 tokens (default threshold)
- Individual block > 500 tokens (default min block size)

**What it means:**
- Large code blocks, logs, or text moved to memory
- Replaced with reference stub: "[Large content stashed to memory: mem_xyz123]"
- Agent can retrieve via `memory_expand(id: "mem_xyz123")` if needed

### 4. Compaction (`role: "compaction"`)

Logged when multiple memories are merged into one.

```json
{
  "ts": "2025-03-01T10:32:00Z",
  "role": "compaction",
  "content": "compacted 2 memories into mem_combined",
  "request": {
    "original_ids": ["mem_1", "mem_2"],
    "new_id": "mem_combined",
    "tags": ["tag1", "tag2"]
  }
}
```

**When it happens:**
- Background memory maintenance
- Manual compaction operations

**What it means:**
- Related memories merged to reduce DB size
- Original memories marked as forgotten
- New combined memory inherits tags and importance

## Usage

All logging happens automatically when context management operations occur. No manual intervention needed.

### Reading Logs

Session logs are in `.inber/logs/<agent-name>/<session-id>/session.jsonl`:

```bash
# View all context management events
cat .inber/logs/main/2025-03-01_103000_a1b2/session.jsonl | jq 'select(.role | IN("summarize", "prune", "stash", "compaction"))'

# Count pruning events
cat session.jsonl | jq 'select(.role == "prune")' | wc -l

# Total tokens freed by pruning
cat session.jsonl | jq 'select(.role == "prune") | .request.tokens_freed' | paste -sd+ | bc
```

### Dashboard Integration

The [inber-party](https://github.com/kayushkin/inber-party) dashboard visualizes these events:

- Timeline showing when each operation occurred
- Token savings graphs
- Memory efficiency metrics

## Configuration

Context management can be tuned via agent config:

```json
{
  "agent": "main",
  "role": "coding",
  "summarize": {
    "turn_threshold": 40,
    "token_threshold": 50000,
    "keep_recent": 10
  },
  "prune": {
    "tool_result_keep_full": 3,
    "tool_result_summary": 10,
    "tool_result_drop": 15,
    "assistant_truncate_age": 10
  },
  "stash": {
    "enabled": true,
    "user_threshold": 1000,
    "assistant_threshold": 1500,
    "min_block_size": 500
  }
}
```

See [context-improvements.md](./context-improvements.md) for full details on configuration options.

## Implementation

- `session/session.go`: Log methods (`LogSummarize`, `LogPrune`, `LogStash`, `LogCompaction`)
- `cmd/inber/engine.go`: Calls log methods when operations occur
- `cmd/inber/summarize.go`: Conversation summarization logic
- `cmd/inber/prune.go`: Message pruning logic
- `cmd/inber/stash.go`: Content stashing logic

## Testing

```bash
go test ./session -v -run TestContextManagement
```

Test verifies all four log types are written correctly with proper metadata.
