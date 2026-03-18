# Inber Data Flow

This document traces the complete data flow from user input to response, showing how components interact during a conversation turn.

## High-Level Flow

```
User Input → Engine → System Prompt → Provider → Tool Execution → Response
```

## Detailed Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         1. USER INPUT                                │
├─────────────────────────────────────────────────────────────────────┤
│  User types message                                                  │
│    ↓                                                                │
│  CLI/Gateway receives input string                                   │
│    ↓                                                                │
│  Creates EngineConfig with model, tools, display hooks              │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                       2. ENGINE.RUNTURN()                            │
├─────────────────────────────────────────────────────────────────────┤
│  📝 Increment turn counter, log turn start                          │
│  📝 Session.LogUser(input) - record to session                      │
│                                                                     │
│  🗜️  STASH: Check if user input > stash threshold                  │
│      → If too large, stash to memory DB and replace with pointer   │
│                                                                     │
│  📋 Add user message to conversation: Messages.append(input)        │
│                                                                     │
│  ✂️  PRUNE: Clean up conversation if too long                      │
│      → Summarize old turns                                         │
│      → Truncate long tool results                                  │
│                                                                     │
│  🧠 BuildSystemPrompt(input) - gather context                       │
│      ├─ Load static memory (identity, preferences, decisions)       │
│      ├─ Load ephemeral metadata (tool summaries, cached results)    │
│      ├─ Generate dynamic tool content (repo_map, recent_files)      │
│      └─ Combine into system blocks                                 │
│                                                                     │
│  🏥 selectModel() - choose healthy model with failover             │
│                                                                     │
│  🔌 Create/update ModelClient for selected model                    │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                    3. PROVIDER ROUTING                               │
├─────────────────────────────────────────────────────────────────────┤
│  if modelClient.IsOpenAI():                                         │
│    ↓                                                               │
│  runOpenAITurn(systemBlocks) ──────────┐                           │
│                                        │                           │
│  else:                                 │                           │
│    ↓                                   │                           │
│  FilterMessagesForAnthropic()          │                           │
│    → Remove OpenAI-specific blocks     │                           │
│    ↓                                   │                           │
│  buildAgent(systemBlocks)              │                           │
│    ↓                                   │                           │
│  Agent.Run(model, messages) ───────────┘                           │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      4. AGENT.RUN() LOOP                             │
├─────────────────────────────────────────────────────────────────────┤
│  📋 Build tool definitions with cache control                       │
│  📋 Create toolMap[toolName] → Tool for execution                   │
│                                                                     │
│  🔄 Main loop (max 50 API calls per turn):                         │
│      ┌─────────────────────────────────────────────────┐           │
│      │  🌐 API call to provider (Anthropic/OpenAI)      │           │
│      │     → Send: system prompt + messages + tools    │           │
│      │     ← Receive: text response + tool calls        │           │
│      │                                                 │           │
│      │  if response.ToolUse != nil:                    │           │
│      │    ↓                                            │           │
│      │    For each tool call:                          │           │
│      │      🔧 Execute tool.Run(ctx, input)            │           │
│      │      🎣 OnToolCall hook (for display)           │           │
│      │      📄 Collect result                          │           │
│      │      🎣 OnToolResult hook (for display)         │           │
│      │      📝 Append tool_result to messages          │           │
│      │    ↓                                            │           │
│      │    Continue loop (more API calls)               │           │
│      │                                                 │           │
│      │  else (text response):                          │           │
│      │    ↓                                            │           │
│      │    🛑 Break loop - final response               │           │
│      └─────────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                       5. TOOL EXECUTION                              │
├─────────────────────────────────────────────────────────────────────┤
│  Each tool implements:                                              │
│    • Name, Description, InputSchema                                │
│    • Run(ctx, input) → (output, error)                            │
│                                                                     │
│  Examples:                                                          │
│    🗂️  repo_map(path, format) → "pkg agent\n*Agent.Run()..."      │
│    📁 recent_files(since) → "5 files: agent.go, engine.go..."      │
│    📖 read_file(path) → [full file content]                        │
│    🔍 search_code(query) → "Found in: agent.go:156"                │
│    💾 write_file(path, content) → "File written"                   │
│    ⚡ exec(cmd) → [command output]                                 │
│                                                                     │
│  Tool results are:                                                  │
│    🎛️  Filtered through ModifyToolResult hook (truncation)        │
│    📝 Added as anthropic.NewToolResultBlock to messages           │
│    🔄 Fed back to LLM for next API call                           │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                    6. RESPONSE PROCESSING                            │
├─────────────────────────────────────────────────────────────────────┤
│  📊 Record model health metrics (latency, success/failure)          │
│  📈 Update token counters and cost tracking                         │
│                                                                     │
│  🗜️  STASH: Check if response > stash threshold                    │
│      → If too large, stash to memory DB                           │
│                                                                     │
│  📝 Session.LogBot(response) - record to session                    │
│                                                                     │
│  🪝 Run workflow hooks (optional post-processing):                 │
│      ├─ Build verification (check for syntax errors)              │
│      ├─ Test verification (run tests after code changes)          │
│      ├─ Git verification (check repo state)                       │
│      └─ Deploy hooks (trigger deployments)                        │
│                                                                     │
│  💾 Save ephemeral memories (tool summaries, context cache)        │
│                                                                     │
│  🔄 Extract long-term memories from conversation                    │
│      → Save decisions, facts, preferences to permanent memory      │
└─────────────────────────────────────────────────────────────────────┘
                                   ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      7. RESPONSE DELIVERY                            │
├─────────────────────────────────────────────────────────────────────┤
│  🎨 Format response for display:                                    │
│      ├─ CLI: Terminal formatting, syntax highlighting             │
│      ├─ Gateway: HTTP streaming, markdown rendering               │
│      └─ Channel: Platform-specific formatting (Discord, WhatsApp) │
│                                                                     │
│  📤 Deliver to user through chosen interface                        │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Components

### Engine (`engine/`)
- **Purpose**: Orchestrates the entire conversation flow
- **Files**: `engine.go`, `turn.go`, `build.go`, `lifecycle.go`
- **Responsibilities**:
  - Turn management and counting
  - System prompt building
  - Conversation pruning and stashing
  - Model selection and failover
  - Provider routing
  - Response processing

### Agent (`agent/`)
- **Purpose**: Handles the LLM conversation loop with tool execution
- **Files**: `agent.go`, `provider.go`, `openai.go`
- **Responsibilities**:
  - API calls to LLM providers
  - Tool call detection and execution
  - Response streaming and hooks
  - Tool result collection

### Memory (`memory/`)
- **Purpose**: Manages persistent context across sessions
- **Responsibilities**:
  - Static memory (identity, preferences, decisions)
  - Ephemeral metadata (tool summaries, cache)
  - Conversation stashing for large content
  - Long-term memory extraction

### Tools (`tools/`)
- **Purpose**: External capabilities the agent can invoke
- **Pattern**: Each tool implements `Tool` interface with `Run(ctx, input) (output, error)`
- **Examples**: File operations, shell execution, repository analysis

## Flow Variations

### With Tools
```
User Input → Engine → Agent → LLM → Tool Call → Tool Execution → LLM → Response
                                 ↑_______________|
```

### Without Tools
```
User Input → Engine → Agent → LLM → Text Response
```

### OpenAI vs Anthropic
```
Engine → modelClient.IsOpenAI() ?
           ├─ Yes: runOpenAITurn()
           └─ No:  FilterMessages + buildAgent + Agent.Run()
```

## Token Flow

1. **Input**: User message tokens
2. **Context**: System prompt tokens (memory + tools)
3. **API**: Request tokens sent to provider
4. **Response**: Response tokens from provider
5. **Tracking**: Session-level accumulation of input/output tokens
6. **Cost**: Calculated based on model pricing

## Error Handling

- **Context Length**: Automatic pruning and re-submission
- **Model Health**: Failover to backup models
- **Tool Errors**: Captured and returned as tool results
- **API Limits**: Exponential backoff and retry
- **Timeout**: Hard caps on API calls per turn (50 max)

## Performance Optimizations

- **Prompt Caching**: Cache control on system prompts and tool definitions
- **Stashing**: Large content moved to memory DB with pointers
- **Pruning**: Old messages and long tool results truncated
- **Streaming**: Real-time response display during generation
- **Tool Summaries**: Cached metadata instead of re-generating

## Memory Management

- **Conversation**: In-memory array of `anthropic.MessageParam`
- **Context Budget**: Dynamic system prompt sizing based on model limits
- **Persistence**: Session and memory stores for long-term retention
- **Cleanup**: Automatic expiration of ephemeral metadata