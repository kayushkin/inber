# Context Loading Explained

## Overview

Inber uses an adaptive context loading system that selects relevant memories for each turn based on:
1. **User query tags** - extracted keywords and categories
2. **Memory importance** - 0.0-1.0 score (higher = more likely to load)
3. **Token budget** - dynamic allocation based on query complexity
4. **Memory size limits** - skip oversized memories (use tools instead)

This document explains what's happening behind the scenes when you chat with an inber agent.

## What Gets Loaded Every Turn

### 1. Always-Load Memories (Fixed)

These memories are marked `always_load = 1` and appear in every turn:

- **Identity** (~500 tokens) - Agent personality, role, and behavior
- **Tool registry** (~300 tokens) - List of available tools with descriptions
- **User preferences** (~50 tokens each) - Your name, preferences, project context

**Logs:**
```
INFO  context: 23 memories, 2347 tokens (min_importance=0.3, budget=50000)
```

The first 3-5 memories are always these fixed blocks.

### 2. Auto-Selected Context (Dynamic)

Based on your query, inber searches memories for relevant context:

**Example query:** "Why isn't the 11k error log being truncated?"

**Auto-tags extracted:**
- `error`, `log`, `truncate`, `truncation`, `file`, `context`

**Memories selected** (in `logs/{agent}/{session}/prompts/system-04-*.md`):
```
system-04-8c21c4d1 (0.8, tags: smart-truncation,design).md     (~800 tokens)
system-05-27325d5f (0.7, tags: tool-hybrid,architecture).md   (~1200 tokens)
system-06-fc030247 (0.6, tags: context-memory,decision).md    (~900 tokens)
...
```

**Token budget calculation:**
- Simple queries ("hi", "yes"): 20K tokens → min_importance=0.5
- Medium queries (10-50 words): 35K tokens → min_importance=0.4
- Complex queries (50+ words): 50K tokens → min_importance=0.3

**MaxChunkSize filter:**
- Memories > 3000 tokens are **skipped** (use `repo_map()` or `read_file()` instead)
- This prevents 10K token repo maps from eating the context budget

### 3. What Gets Excluded

**ExcludeTags** (always filtered out):
- `session-summary` - noisy, low-value conversation summaries
- `repo-map` - too large (10K+ tokens), use `repo_map()` tool instead
- `code-introspection` - stale generated content

**Size limits:**
- Memories > 3K tokens (skipped)
- File references > 3K tokens (lazy-loaded, use `memory_expand()`)

**Low importance:**
- If token budget is tight, memories below min_importance are skipped
- Example: on simple query, only memories with importance ≥ 0.5 load

## How to Debug Context Issues

### Check Prompt Breakdown Files

Each turn generates:
```
logs/{agent}/{session_id}/prompts/
├── system.md                   ← INDEX of all blocks with token counts
├── system-01-identity.md       ← Always-load (identity)
├── system-02-memory-instructions.md
├── system-03-tool-registry.md
├── system-04-8c21c4d1 (...).md ← Auto-selected context memories
├── ...
├── tools.md                    ← Tool definitions
└── turn-N.md                   ← Messages for this turn
```

**system.md shows:**
```markdown
| # | Block | Tokens (est) |
|---|-------|--------------|
| 1 | [identity](system-01-identity.md) | ~534 |
| 2 | [memory-instructions](system-02-memory-instructions.md) | ~287 |
| 3 | [tool-registry](system-03-tool-registry.md) | ~312 |
| 4 | [8c21c4d1 (0.8, tags: smart-truncation,design)](system-04-...) | ~823 |
| 5 | [27325d5f (0.7, tags: tool-hybrid,architecture)](system-05-...) | ~1187 |
...
**Total:** ~15234 tokens
```

### Check Logs

**Context selection:**
```
INFO  context: 23 memories, 15234 tokens (min_importance=0.3, budget=50000)
```
- 23 memories loaded (3 always-load + 20 auto-selected)
- 15K tokens used out of 50K budget
- Minimum importance threshold: 0.3 (lower = more permissive)

**Excluded memories:**
```
DEBUG excluded 2 memories (tags: repo-map, session-summary)
DEBUG excluded 5 memories (size > 3000 tokens)
```

### SQLite Queries

**Find large memories:**
```bash
sqlite3 .inber/memory.db "
SELECT id, tokens, tags 
FROM memories 
WHERE tokens > 3000 
ORDER BY tokens DESC 
LIMIT 10;
"
```

**Find always-load memories:**
```bash
sqlite3 .inber/memory.db "
SELECT id, content, tokens 
FROM memories 
WHERE always_load = 1;
"
```

**Find memories by tag:**
```bash
sqlite3 .inber/memory.db "
SELECT m.id, m.tokens, GROUP_CONCAT(mt.tag) as tags
FROM memories m
LEFT JOIN memory_tags mt ON m.id = mt.memory_id
GROUP BY m.id
HAVING tags LIKE '%repo-map%';
"
```

## Common Issues

### Issue: Agent forgets recent conversations

**Cause:** Conversation summarized after 70 turns, old details compressed.

**Solution:** Check `logs/{session}/session.jsonl` for summarize events:
```json
{"role":"summarize","content":"summarized 15 turns → 250 token summary"}
```

Use `memory_search` to retrieve full conversation from memory store.

### Issue: 10K token repo map loading every turn

**Cause:** Repo map has tags that match user queries.

**Solution:** ✅ Already fixed! Repo maps now excluded via:
```go
ExcludeTags: []string{"repo-map", "code-introspection"}
```

Use `repo_map()` tool instead of auto-loading.

### Issue: Agent doesn't have file contents

**Cause:** Files are lazy-loaded (summary only, 50 tokens).

**Solution:** Use `read_file()` tool to load full content on-demand. This is by design - files change frequently, storing full content would go stale.

### Issue: Important memory not loading

**Cause:** Importance too low or no matching tags.

**Solution:**
1. Check memory importance: `sqlite3 .inber/memory.db "SELECT id, importance, tags FROM memories WHERE id = 'xxx';"`
2. If importance < 0.5, it may be filtered out on simple queries
3. Update importance: `memory_save` with higher importance
4. Add relevant tags to improve matching

## Best Practices

### Memory Importance Guidelines

- **1.0** - Critical identity, user preferences (always load)
- **0.8** - Key architectural decisions, important patterns
- **0.6** - Design documents, medium-priority facts
- **0.4** - Session summaries, secondary context
- **0.2** - Temporary notes, experimental ideas
- **0.0** - Ephemeral data (expires automatically)

### Tagging Strategy

**Good tags** (semantic, discoverable):
- `decision`, `architecture`, `design`, `preference`, `fact`
- `context-memory`, `smart-truncation`, `tool-hybrid`
- `user`, `agent`, `session`

**Bad tags** (too specific, not discoverable):
- `2026-03-01`, `turn-42`, `temp`, `misc`

### When to Use Tools vs Auto-Load

**Auto-load** (store in memory):
- Small facts (< 500 tokens)
- Decisions and preferences
- Architectural patterns
- User context

**Tools** (lazy-load on-demand):
- File contents (use `read_file()`)
- Repo structure (use `repo_map()`)
- Recent changes (use `recent_files()`)
- Large documents (> 3K tokens)

## Future Improvements

1. **LLM-based context selection** - Ask Haiku which memories to load
2. **Adaptive token budgets** - Adjust based on conversation complexity
3. **Memory clustering** - Group related memories for batch loading
4. **Reference graphs** - Track memory relationships for better retrieval
5. **Session-aware importance** - Boost relevance for current project context
