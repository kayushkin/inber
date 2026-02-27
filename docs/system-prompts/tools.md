# Available Tools

Inber provides several categories of tools that agents can use. Your agent's tool access is defined in `agents.json`.

## Built-in Tools

### File Operations

#### `read_file`
Read the contents of a file.

**Parameters:**
- `path` (string, required): File path to read

**Returns:** File contents as text

**Example:**
```json
{
  "path": "main.go"
}
```

---

#### `write_file`
Create or overwrite a file with new content.

**Parameters:**
- `path` (string, required): File path to write
- `content` (string, required): Content to write

**Returns:** Confirmation message

**Example:**
```json
{
  "path": "test.txt",
  "content": "Hello, world!"
}
```

---

#### `edit_file`
Make precise edits to a file by replacing exact text matches.

**Parameters:**
- `path` (string, required): File path to edit
- `old_text` (string, required): Exact text to find and replace (must match exactly)
- `new_text` (string, required): New text to replace with

**Returns:** Confirmation message or error if text not found

**Example:**
```json
{
  "path": "main.go",
  "old_text": "func oldName() {",
  "new_text": "func newName() {"
}
```

**Note:** `old_text` must match exactly (including whitespace). Use `read_file` first to see exact formatting.

---

#### `list_files`
List files in a directory with optional pattern filtering.

**Parameters:**
- `path` (string, required): Directory path to list
- `pattern` (string, optional): Glob pattern to filter results (e.g., "*.go")
- `recursive` (boolean, optional): Whether to list recursively (default: false)

**Returns:** List of file paths

**Example:**
```json
{
  "path": ".",
  "pattern": "*.go",
  "recursive": true
}
```

---

### Shell Operations

#### `shell`
Execute shell commands.

**Parameters:**
- `command` (string, required): Shell command to execute

**Returns:** Command output (stdout + stderr)

**Example:**
```json
{
  "command": "go test ./..."
}
```

**Note:** 
- Always use absolute paths or be aware of current working directory
- Commands run in a real shell with full environment access
- Long-running commands may timeout

---

## Memory Tools

### `memory_search`
Search persistent memories by semantic similarity.

**Parameters:**
- `query` (string, required): Search query text
- `limit` (integer, optional): Maximum results (default: 10)

**Returns:** Ranked list of relevant memories with metadata

**Example:**
```json
{
  "query": "authentication bug",
  "limit": 5
}
```

**Use cases:**
- Recall past decisions or learnings
- Find relevant context from previous sessions
- Look up user preferences or project-specific information

---

### `memory_save`
Store a new memory for persistent recall across sessions.

**Parameters:**
- `content` (string, required): Memory content to store
- `tags` (array of strings, optional): Tags for categorization
- `importance` (number, optional): Score 0-1 (default: 0.5)
- `source` (string, optional): Source: "user", "agent", "system" (default: "agent")

**Returns:** Memory ID

**Example:**
```json
{
  "content": "User prefers snake_case for variable names in Go projects",
  "tags": ["preference", "code-style", "go"],
  "importance": 0.8,
  "source": "user"
}
```

**When to save memories:**
- User preferences or decisions
- Important project context or constraints
- Lessons learned from debugging
- Key facts about the codebase
- Solutions to recurring problems

**Importance guidelines:**
- 0.9-1.0: Critical information (security keys, user requirements)
- 0.7-0.8: Important preferences or architectural decisions
- 0.5-0.6: Useful context or patterns
- 0.3-0.4: Minor details or temporary notes
- 0.0-0.2: Low-priority observations

---

### `memory_expand`
Retrieve full content of a memory by ID.

**Parameters:**
- `id` (string, required): Memory ID to retrieve

**Returns:** Full memory with metadata (created date, access count, etc.)

**Example:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Use cases:**
- Expand compacted/summarized memories
- Get full context when search results are truncated
- Review memory history and access patterns

---

### `memory_forget`
Mark a memory as forgotten (soft delete).

**Parameters:**
- `id` (string, required): Memory ID to forget

**Returns:** Confirmation message

**Example:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Note:** Memory remains in storage but won't appear in search results. Useful for outdated or incorrect information.

---

## Tool Access Patterns

### By Agent Type

**Coder agents** typically have:
- All file operations (read, write, edit, list)
- Shell access
- All memory tools

**Researcher agents** typically have:
- Read-only file operations (read, list)
- Memory search and expand (but not save)
- No shell access

**Orchestrator agents** typically have:
- Read-only file operations
- All memory tools
- spawn_agent tool (coming soon)

### Best Practices

1. **Always read before editing**: Use `read_file` to see exact formatting before using `edit_file`
2. **Memory hygiene**: Save important context, forget outdated info
3. **Tag your memories**: Use specific tags for better search results
4. **Shell safety**: Check command syntax before executing destructive operations
5. **Importance scoring**: Be thoughtful about memory importance to keep high-value content accessible

---

## Tool Scoping

Your available tools are defined in your agent configuration (`agents.json`). If you attempt to use a tool you don't have access to, you'll receive an error.

Check your agent config to see which tools you can use.
