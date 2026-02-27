# Session Management

Inber provides **persistent sessions** that allow conversations to resume exactly where they left off, with full context continuity across multiple interactions.

## Session Lifecycle

```
┌─────────────────────────────────────────┐
│         Session Creation                │
├─────────────────────────────────────────┤
│ 1. Load or create session file          │
│ 2. Load conversation history (JSONL)    │
│ 3. Auto-load context (files, memories)  │
│ 4. Initialize agent with system prompt  │
└─────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────┐
│         Turn Execution                  │
├─────────────────────────────────────────┤
│ 1. User message → append to session     │
│ 2. Build context from current state     │
│ 3. Call LLM with full history + context │
│ 4. Log response (including tool calls)  │
│ 5. Execute tools, log results           │
│ 6. Return final response to user        │
└─────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────┐
│         Session Close                   │
├─────────────────────────────────────────┤
│ 1. Flush logs to disk                   │
│ 2. Update session metadata              │
│ 3. Save high-importance context to      │
│    memory (future feature)              │
└─────────────────────────────────────────┘
```

## Session Types

### 1. Default Session (Persistent)

The **default** behavior - conversations continue across multiple CLI invocations:

```bash
$ ./inber
You: Write a hello world program
Agent: [writes code]

$ ./inber  # Later, in a new terminal
You: Now add error handling
Agent: [adds error handling to the previous code]
```

**Session file:** `.inber/sessions/{agent}/default.jsonl`

**Characteristics:**
- Automatically resumes
- Full conversation history preserved
- Context includes files and memories from previous turns
- No special flags needed

---

### 2. New Session (Fresh Start)

Start a **brand new** conversation, ignoring previous history:

```bash
$ ./inber --new
You: Write a hello world program
Agent: [writes code with no knowledge of previous conversations]
```

**Session file:** `.inber/sessions/{agent}/{timestamp}.jsonl`

**Use cases:**
- Completely unrelated task
- Previous session is too cluttered
- Want to experiment without affecting default session

**Note:** The default session remains untouched - you can return to it later.

---

### 3. Detached Session (One-Off)

Execute a **single command** without affecting the default session:

```bash
$ ./inber --detach "What's the weather?"
Agent: [responds without loading or modifying default session]
```

**Session file:** Temporary (`.inber/sessions/{agent}/detached-{uuid}.jsonl`)

**Use cases:**
- Quick one-off queries
- Testing without polluting session history
- Automated scripts that shouldn't interfere with interactive sessions

**Characteristics:**
- No history loaded
- Response not saved to default session
- Minimal context (only identity + high-importance memories)
- Session file auto-deleted after completion

---

## Session Storage

### JSONL Format

Sessions are stored as **JSON Lines** (one JSON object per line):

```jsonl
{"type":"request","timestamp":"2024-02-24T10:00:00Z","model":"claude-sonnet-4-5","messages":[...]}
{"type":"response","timestamp":"2024-02-24T10:00:05Z","id":"msg_abc123","content":[...],"usage":{...}}
{"type":"tool_call","timestamp":"2024-02-24T10:00:06Z","name":"read_file","input":{...}}
{"type":"tool_result","timestamp":"2024-02-24T10:00:07Z","name":"read_file","output":"..."}
```

### Log Structure

Each session log contains:

1. **Request**: User message + full message history
2. **Response**: Assistant message (including thinking blocks if enabled)
3. **Tool calls**: Each tool invocation
4. **Tool results**: Output from each tool

**Full request payloads** are logged, allowing complete reproduction of any turn.

---

## Directory Structure

```
.inber/
└── sessions/
    ├── coder/
    │   ├── default.jsonl          # Default persistent session
    │   ├── 2024-02-24-100000.jsonl  # --new session
    │   └── 2024-02-24-110000.jsonl  # Another --new session
    ├── researcher/
    │   └── default.jsonl
    └── task-manager/
        └── default.jsonl
```

Each **agent** has its own session directory, allowing per-agent conversation continuity.

---

## Message History

### Conversation Structure

Messages are structured as:

```go
type Message struct {
    Role    string  // "user" or "assistant"
    Content string  // Message text (or tool results)
}
```

**Example flow:**
```
[user]:      "Write a hello world program"
[assistant]: "Here's a hello world program: [uses write_file tool]"
[user]:      "Now add error handling"
[assistant]: "I'll add error handling: [uses edit_file tool]"
```

The **full history** is sent to the LLM on each turn, so the agent always has complete context.

---

## Context Loading

### At Session Start

1. **Load session file** (if exists)
2. **Parse conversation history**
3. **Auto-load context:**
   - Identity (system prompt)
   - Recent files
   - Repo map (if applicable)
   - High-importance memories (score > 0.7)
4. **Rebuild context store** with all sources

### Between Turns

Context is **rebuilt** for each turn to incorporate:
- New file modifications
- New memories saved
- Tool outputs from previous turn

---

## Session Continuity Guarantees

### What Persists

✅ **Conversation history** - Full message log  
✅ **Context references** - File paths, memory IDs  
✅ **Tool outputs** - Previous tool call results  
✅ **Session metadata** - Model used, timestamps  

### What Doesn't Persist

❌ **In-memory state** - Agent object is recreated each run  
❌ **Temporary files** - Unless explicitly saved  
❌ **Environment variables** - Re-read from .env each time  

**Implications:**
- Agent must rely on session logs and memory system for state
- Don't store critical info in variables - save to memory or workspace
- File system changes persist naturally (files written remain)

---

## Best Practices

### For Interactive Use

1. **Use default session** for ongoing work
2. **Use --new** when switching projects or contexts
3. **Use --detach** for quick questions

### For Agents

1. **Check conversation history** before asking questions already answered
2. **Reference previous work**: "As I mentioned earlier..." when appropriate
3. **Save important decisions to memory** for cross-session recall
4. **Use workspace files** for structured state (task queues, etc.)

### For Long Sessions

**Problem:** Default session grows indefinitely → context overflow

**Solutions (planned):**
1. **Conversation pruning** - Automatically trim old messages based on token budget
2. **Session compaction** - Summarize old turns into memory
3. **Manual reset** - User can archive and start fresh

**Workaround (current):**
- Use `--new` to start fresh when session is too long
- Use `memory_save` to preserve key information before resetting

---

## Session Metadata

Each session tracks:

```go
type Session struct {
    ID          string    // Unique session ID
    Agent       string    // Agent name
    Model       string    // Model used
    StartedAt   time.Time // Session creation
    LastTurnAt  time.Time // Most recent interaction
    TurnCount   int       // Number of turns
    TotalTokens int       // Cumulative token usage
    TotalCost   float64   // Cumulative cost ($)
}
```

Metadata is logged at session start and updated after each turn.

---

## Future Enhancements

### Session Management Commands

```bash
inber sessions list                # Show all sessions
inber sessions show <id>           # View session details
inber sessions delete <id>         # Delete a session
inber sessions export <id>         # Export to markdown
inber sessions switch <id>         # Switch default session
```

### Session Branching

```bash
inber --branch "experiment"        # Create named branch from default
```

Allows experimentation without losing main conversation thread.

### Cross-Agent Session Sharing

```bash
inber --agent coder --continue-from researcher:session-123
```

Continue a conversation started by another agent (with context inheritance).

### Session Compression

- Automatic summarization of old turns
- Replace old messages with summaries to save tokens
- Keep full logs on disk for reference

---

## Example Workflows

### Daily Development

```bash
# Morning - resume yesterday's work
$ ./inber
You: Where did we leave off?
Agent: Yesterday we were implementing the authentication system...

# Afternoon - switch to different task
$ ./inber --new
You: Let's work on the API documentation now
Agent: [starts fresh, focused on docs]

# Evening - return to authentication work
$ ./inber  # Back to default session
You: Let's finish that auth implementation
Agent: Right, we had the OAuth flow partially complete...
```

### Quick Questions

```bash
# Don't want to pollute your main session
$ ./inber --detach "What's the syntax for Go interfaces?"
Agent: [explains interfaces]

$ ./inber  # Back to main session, no history of the quick question
You: [continues where you left off]
```

---

## Debugging Sessions

### View Raw Session Log

```bash
cat .inber/sessions/coder/default.jsonl | jq
```

### Extract Conversation Only

```bash
cat .inber/sessions/coder/default.jsonl | jq 'select(.type=="request" or .type=="response")'
```

### Calculate Token Usage

```bash
cat .inber/sessions/coder/default.jsonl | jq -s 'map(select(.usage) | .usage.input_tokens + .usage.output_tokens) | add'
```

---

## Session Persistence Guarantees

**Inber guarantees:**
1. Session logs are **flushed to disk** after each turn
2. Incomplete turns (crashes) are **logged up to the point of failure**
3. Session files use **append-only** writes (no data loss from concurrent access)

**You can safely:**
- Kill inber mid-turn - restart will recover
- Run multiple inber instances - sessions are isolated by agent
- Edit session files manually - changes picked up on next load (risky but possible)
