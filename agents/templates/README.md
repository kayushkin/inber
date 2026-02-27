# Agent Templates

This directory contains **template agent identities** that can be used as starting points for creating new specialized agents.

## Usage

### By Task-Manager Agent

The task-manager can use these templates to dynamically create new agents:

1. Copy template to `agents/{new-agent-name}.md`
2. Customize the identity for the specific use case
3. Add configuration to `agents.json`
4. Delegate to the new agent

### By Developers

Use these as starting points when defining new agent types:

```bash
cp agents/templates/coder.md agents/my-specialist.md
# Edit my-specialist.md to customize
# Add entry to agents.json
```

## Available Templates

### coder.md
- **Best for:** Software engineering, implementation, debugging
- **Tools:** Full access (shell + all file ops)
- **Context:** Code-focused, large budget
- **Customize for:** 
  - Language-specific coders (golang-specialist, python-expert)
  - Domain-specific engineers (backend-dev, frontend-dev)
  - Tool-specific developers (docker-specialist, k8s-admin)

### researcher.md
- **Best for:** Analysis, investigation, documentation
- **Tools:** Read-only access
- **Context:** Research-focused, medium budget
- **Customize for:**
  - Security auditors (vulnerability scanning)
  - Performance analyzers (profiling, optimization)
  - Documentation specialists (API docs, guides)

### orchestrator.md
- **Best for:** Complex multi-step coordination
- **Tools:** spawn_agent (when available), read access
- **Context:** Coordination-focused, smaller budget
- **Customize for:**
  - Project managers (tracking, reporting)
  - Build coordinators (CI/CD orchestration)
  - Release managers (deployment workflows)

## Creating Custom Agents

### 1. Define Identity (Markdown)

```markdown
# My Specialist Agent

You are a [specialist type] focused on [specific domain].

## Your Role
[What you do, what you're good at]

## Tools Available
[Which tools you have and how to use them]

## Best Practices
[Domain-specific guidelines]
```

### 2. Configure Settings (JSON)

Add to `agents.json`:

```json
"my-specialist": {
  "name": "my-specialist",
  "role": "brief role description",
  "model": "claude-sonnet-4-5",
  "thinking": 0,
  "tools": [
    "read_file",
    "list_files"
  ],
  "context": {
    "tags": ["domain", "specific"],
    "budget": 30000
  }
}
```

### 3. Tool Selection Guide

**Read-only access:**
```json
"tools": ["read_file", "list_files", "memory_search", "memory_expand"]
```

**Code modification:**
```json
"tools": ["read_file", "write_file", "edit_file", "list_files"]
```

**Full system access:**
```json
"tools": ["shell", "read_file", "write_file", "edit_file", "list_files"]
```

**Memory tools:**
- `memory_search` - Search memories
- `memory_save` - Create new memories
- `memory_expand` - Get full memory details
- `memory_forget` - Mark memory as irrelevant

### 4. Budget Guidelines

- **Large (50k+ tokens):** Full-stack work, needs lots of context
- **Medium (30-40k tokens):** Focused tasks, moderate context
- **Small (20k tokens):** Narrow specialists, minimal context

## Example Specializations

### Security Auditor

```markdown
# Security Auditor

You are a security specialist focused on finding vulnerabilities.

Analyze code for:
- SQL injection risks
- XSS vulnerabilities
- Authentication bypasses
- Secrets in code
- Insecure dependencies

Tools: Read-only (no modifications allowed during audit)
```

```json
"security-auditor": {
  "name": "security-auditor",
  "role": "security vulnerability analyst",
  "model": "claude-sonnet-4-5",
  "thinking": 4096,
  "tools": ["read_file", "list_files", "memory_search"],
  "context": {
    "tags": ["security", "vulnerabilities", "audit"],
    "budget": 40000
  }
}
```

---

### Test Engineer

```markdown
# Test Engineer

You are a testing specialist focused on comprehensive test coverage.

Your responsibilities:
- Write unit tests
- Write integration tests
- Identify edge cases
- Ensure test coverage
- Maintain test quality

Tools: Full file access + shell (for running tests)
```

```json
"test-engineer": {
  "name": "test-engineer",
  "role": "testing and quality assurance specialist",
  "model": "claude-sonnet-4-5",
  "thinking": 0,
  "tools": ["shell", "read_file", "write_file", "edit_file", "list_files"],
  "context": {
    "tags": ["tests", "quality", "coverage"],
    "budget": 45000
  }
}
```

---

### Documentation Writer

```markdown
# Documentation Writer

You are a documentation specialist focused on clear, accurate docs.

Create:
- API documentation
- User guides
- Architecture docs
- Code comments
- README files

Tools: Read code (understand), write docs (markdown files)
```

```json
"doc-writer": {
  "name": "doc-writer",
  "role": "technical documentation specialist",
  "model": "claude-sonnet-4-5",
  "thinking": 0,
  "tools": ["read_file", "write_file", "edit_file", "list_files", "memory_search"],
  "context": {
    "tags": ["documentation", "markdown", "code"],
    "budget": 35000
  }
}
```

## Best Practices

1. **Keep identities focused** - Specialists are more effective than generalists
2. **Match tools to role** - Don't give shell access if not needed
3. **Set appropriate budgets** - More context = slower + costlier
4. **Use tags effectively** - Help context system load relevant files
5. **Enable thinking for complex roles** - Security audits, architecture decisions
6. **Document tool usage** - Explain how each tool supports the role
7. **Save successful patterns** - Working agent configs can be templates for similar needs

## Dynamic Creation

The task-manager can create agents on-the-fly by:

1. Detecting a need for a new specialist type
2. Selecting an appropriate template
3. Writing customized identity file
4. Updating agents.json with configuration
5. Delegating to the new agent immediately

This allows the system to adapt to new task types without pre-defining every possible specialist.
