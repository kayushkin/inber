# Workspace System

Inber uses a **workspace directory** at `.inber/workspace/{agent}/` to store agent-specific, editable files that persist across sessions.

## Directory Structure

```
.inber/
├── memory.db                  # Shared memory database
└── workspace/
    ├── coder/                 # Coder agent workspace
    │   ├── system.md          # Editable system prompt
    │   ├── preferences.md     # User preferences
    │   └── notes.txt          # Scratch notes
    ├── researcher/            # Researcher agent workspace
    │   ├── system.md
    │   └── sources.md
    └── task-manager/          # Task manager workspace
        ├── system.md
        ├── task-queue.json    # Active tasks
        └── delegation-log.md  # Delegation history
```

## Purpose

The workspace provides:

1. **Editable System Prompts**
   - Override default identity from `agents/{name}.md`
   - Users can customize agent behavior without editing code
   - Changes persist across sessions

2. **Agent-Specific State**
   - Task queues, scratch notes, preferences
   - Not in memory system (too structured or transient)
   - Direct file I/O, not searched semantically

3. **User Customization**
   - Users can edit files between sessions
   - Agents can read/write workspace files
   - Combine manual and automated configuration

## File Types

### system.md (Optional Override)

If `.inber/workspace/{agent}/system.md` exists, it **replaces** the default system prompt from `agents/{agent}.md`.

**Use cases:**
- User wants to customize agent behavior
- Temporary personality/focus changes
- Project-specific instructions

**Example:**
```markdown
# Coder Agent (Custom)

You are a coding specialist for this project specifically.

## Project-Specific Rules
- Always use snake_case for variables
- Write tests before implementation
- Use error wrapping with fmt.Errorf

## Your Tools
[Original tools still apply based on agents.json]
```

**Loading priority:**
1. `.inber/workspace/{agent}/system.md` (if exists)
2. `agents/{agent}.md` (default)

---

### preferences.md (User-Editable)

Stores user preferences for this specific agent.

**Example:**
```markdown
# My Coder Preferences

- Verbose error messages
- Always run tests after writing code
- Use `golang` for Go code blocks, not `go`
- Prefer functional style over imperative
```

Agents should read this file at session start and incorporate preferences.

---

### Task/State Files

Agents can create arbitrary files for state management:

- `task-queue.json` - Pending tasks
- `delegation-log.md` - History of sub-agent spawns
- `notes.txt` - Temporary scratch space
- `bookmarks.json` - Saved locations or references

**These files are NOT automatically loaded** - agents must explicitly read them.

---

## File Operations

### Reading Workspace Files

Use the standard `read_file` tool:

```json
{
  "path": ".inber/workspace/coder/preferences.md"
}
```

### Writing Workspace Files

Use the standard `write_file` or `edit_file` tools:

```json
{
  "path": ".inber/workspace/coder/task-queue.json",
  "content": "{\"tasks\": [...]}"
}
```

### Initialization

Workspace directories are **automatically created** when an agent starts if they don't exist. No setup required.

---

## Workspace vs Memory

| Feature | Workspace | Memory |
|---------|-----------|--------|
| **Storage** | Files in `.inber/workspace/` | SQLite database |
| **Structure** | Arbitrary (Markdown, JSON, text) | Structured (Memory objects) |
| **Search** | No semantic search | TF-IDF semantic search |
| **Access** | Explicit file reads | Auto-loaded by importance |
| **Scope** | Agent-specific | Shared across all agents |
| **Editing** | Users can edit files manually | Only via tools or API |

**When to use workspace:**
- Structured data (JSON configs, task queues)
- User-editable preferences
- Large text blocks (not suitable for memory)
- Agent-specific state

**When to use memory:**
- Semantic recall ("remember when...")
- Cross-session knowledge
- Importance-ranked information
- Auto-loading high-value context

---

## Best Practices

### For Agent Developers

1. **Document workspace files**: Explain what each file does
2. **Initialize on first use**: Check if file exists, create with defaults
3. **Validate contents**: Handle missing or corrupted files gracefully
4. **Don't pollute**: Only create files you actually need

### For Users

1. **Edit cautiously**: Malformed files can break agent functionality
2. **Use Markdown for notes**: Easy to read and edit
3. **Use JSON for structured data**: Task queues, configs
4. **Keep backups**: Workspace files are not version-controlled by default

### For Task-Manager Agent

The task-manager uses workspace heavily:

**task-queue.json:**
```json
{
  "tasks": [
    {
      "id": "550e8400-...",
      "description": "Implement user authentication",
      "status": "in-progress",
      "assigned_to": "coder",
      "priority": "high",
      "created": "2024-02-24T10:00:00Z"
    }
  ]
}
```

**delegation-log.md:**
```markdown
# Delegation History

## 2024-02-24 10:00 - Task: Implement authentication
- Delegated to: coder
- Reason: Full-stack implementation needed
- Status: In progress
```

---

## Auto-Loading System Prompts

At agent initialization, inber checks:

```python
1. Does .inber/workspace/{agent}/system.md exist?
   YES → Use it as system prompt
   NO  → Use agents/{agent}.md

2. Load into context with priority 1.0, tags ["identity"]
```

This allows users to override agent behavior without modifying source files.

---

## Directory Permissions

- **Agents**: Full read/write access to their own workspace
- **Users**: Full access (can edit files manually between sessions)
- **Cross-agent access**: Agents can read other agents' workspaces (but shouldn't without good reason)

---

## Future Enhancements

### Workspace Templates

Pre-populate workspaces with common files:

```
agents/templates/coder/
├── system.md.template
├── preferences.md.template
└── README.md
```

### Workspace Versioning

- Git integration for workspace changes
- Undo/redo for agent-made edits
- Diff views for debugging

### Shared Workspace

```
.inber/workspace/shared/
├── project-glossary.md
├── team-preferences.md
└── common-patterns.md
```

For cross-agent shared knowledge (not suitable for memory system).

### Workspace Cleanup

- Auto-delete old workspace files
- Archive completed task logs
- Compress large workspace directories

---

## Example: Coder Agent Workflow

**Session Start:**
1. Load `.inber/workspace/coder/system.md` (or default)
2. Read `.inber/workspace/coder/preferences.md`
3. Check for `.inber/workspace/coder/notes.txt` from last session

**During Work:**
1. Save temporary notes to workspace
2. Update task status in `task-queue.json` (if using)

**Session End:**
1. Write summary to `notes.txt`
2. Clean up completed task entries
