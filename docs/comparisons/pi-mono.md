# Pi-Mono Comparison

**Last reviewed**: 2026-10-29 against earendil-works/pi@e7a0df2 (2026-10-29).

**Project**: [pi](https://github.com/earendil-works/pi), formerly `badlogic/pi-mono` (the old URL redirects) by Mario Zechner  
**Language**: TypeScript/Node.js  
**Focus**: Small, extensible agent harness with a multi-provider LLM library  

## Architecture Overview

Pi is a monorepo of separate packages. The npm scope moved from `@mariozechner` to `@earendil-works` (version 1.4.0 at review time). The packages now are:

- `@earendil-works/pi-ai` — Unified multi-provider LLM API
- `@earendil-works/pi-agent-core` — Agent runtime with tool calling and state
- `@earendil-works/pi-coding-agent` — The `pi` CLI
- `@earendil-works/pi-tui` — Terminal UI library
- `@earendil-works/pi-durable` — Durable conversation, task and document runtime
- `@earendil-works/chord`, `pi-telemetry`, plus `mcp`, `codemode`, `env` and `evals` packages

The web UI, Slack bot (`pi-mom`) and vLLM pod packages listed in earlier versions of this doc are gone from the monorepo; Slack and chat work moved to the separate `earendil-works/pi-chat` repo. Pi says it skips sub-agents and plan mode on purpose, and leaves them to extensions and packages.

## What They Do Well

### 1. **Clean Provider Abstraction**
In pi-ai each provider owns its model catalog, its auth (API key, stored credential, OAuth) and its stream code. An app builds a `Models` collection and registers providers on it:

```typescript
const models = createModels({ credentials: myStore });
models.setProvider(anthropicProvider());
```

Provider factories load their SDK lazily, on the first request to a model of that API. The older global `registerApiProvider` / `getApiProvider` registry that this doc first described still exists, but only in a `pi-ai/compat` entry point marked for deletion. About 40 providers ship built in, among them OpenAI, Anthropic, Google, Bedrock, Azure, Mistral, DeepSeek, Groq and any OpenAI-compatible server.

### 2. **Package Modularity**
Each package has a clear, single job:
- **pi-ai**: LLM abstraction, no agent logic
- **pi-agent-core**: agent logic, no provider coupling
- **pi-coding-agent**: CLI application layer

The agent package imports only the abstract types from pi-ai, never provider SDKs.

### 3. **OAuth & Authentication Abstraction**
Every provider carries its own auth, with `login`, `refresh` and `toAuth` behind one `OAuthAuth` interface, and a pluggable `CredentialStore` that refreshes tokens under a lock so two processes cannot both refresh. Subscription sign-in covers:

- **Anthropic Claude Pro/Max**, through pi's own OAuth flow. To be accepted, pi makes the requests look like Claude Code's: the Claude Code identity system block, Claude Code's tool names, and a `claude-cli` user agent. Anthropic's terms reserve subscription OAuth for its own apps, and since 2026-04-04 Anthropic bills third-party use of it as extra usage. Pi warns users about this.
- **OpenAI "Codex (legacy)"**, a ChatGPT Plus/Pro subscription posting to `chatgpt.com/backend-api/codex/responses`.
- **OpenAI "Sign in with ChatGPT"** (since v0.99.0, 2026-09-29), which shares the ChatGPT sign-in token and calls `api.openai.com/v1`.
- GitHub Copilot, OpenRouter, and a few others.

### 4. **Tool System**
Tools are self-contained and provider-agnostic. The agent core runs tools without knowing about LLM details.

## What Inber Could Adopt

### 1. **Provider Interface Pattern**
Inber has had an `agent.Provider` interface since this doc was first written (`agent/provider.go`), but it is not the provider-neutral one proposed here. Its methods take `*anthropic.MessageNewParams` and return `*anthropic.Message`, and `AnthropicProvider` is its only implementation (built in `engine/build.go`, `agent/agent.go` and `agent/registry/registry.go`).

OpenAI models do not go through it. They run in a second turn loop, `runOpenAITurn` in `engine/turn_openai.go`, which converts inber's Anthropic-shaped tools and messages to chat completions (`agent/openai_conversion.go`). When the agent is set to use the ChatGPT subscription, `agent/codex_subscription.go` converts those chat-completion requests again into the Responses API that `chatgpt.com/backend-api/codex/responses` speaks. So inber reaches two providers, but through two loops that have to be kept in step by hand, not one interface.

A neutral interface along these lines is still the step that would let one loop serve both:

```go
type Provider interface {
    Complete(ctx context.Context, messages []Message, tools []Tool, options *Options) (*Response, error)
    Stream(ctx context.Context, messages []Message, tools []Tool, options *Options) (<-chan StreamEvent, error)
}
```

Non-test Go code outside `docs/` still has about 370 references to `anthropic.` (368 counted on 2026-10-29).

### 2. **Package Separation**
Inber now has `agent/` (model calls, tools, message conversion), `engine/` (per-session turn loop), `server/` (multi-session HTTP server) and `cmd/inber-server`. The CLI moved to its own repo, `inber-cli`. What is still missing is a provider package that `agent/` depends on only through neutral types.

### 3. **Registry Pattern**
Use a registry for providers instead of fixed clients:

```go
type ProviderRegistry struct {
    providers map[string]Provider
}

func (r *ProviderRegistry) Register(name string, p Provider)
func (r *ProviderRegistry) Get(name string) Provider
```

Today `engine/model_client.go` picks between an Anthropic client and an OpenAI-compatible client, and model-store holds the provider rows.

### 4. **Unified Message Types**
Define inber-native message types that providers convert to their own formats, rather than passing `anthropic.MessageParam` through the codebase. The engine's history is still `[]anthropic.MessageParam`, and the OpenAI path converts from it on every call.

## Key Differences

### Inber Advantages
- **Go ecosystem**: Single binary, fast startup, easy deployment
- **Server, not CLI**: inber runs as a long-lived multi-session server with a message bus and llm-bridge routes (`server/api_bridge.go`); pi is a CLI you embed or drive over RPC
- **Memory system**: More involved conversation memory, stashing and summarization
- **Limits and modes**: per-session cost, token, turn and time limits and observe/assist/autonomous modes (`guard/`); pi has no permission system and points users to containers
- **Egress redaction**: every provider client strips live secrets from outgoing requests (`redact/`, `agent/redaction.go`)

### Pi Advantages
- **Provider flexibility**: About 40 built-in providers behind one interface
- **Modular design**: Cleaner package boundaries
- **OAuth infrastructure**: Login, refresh and a shared credential store for five subscription or OAuth providers. Inber does no sign-in or refresh of its own: for OpenAI it reads the Codex CLI's auth file and leaves refresh to that CLI, and for Anthropic it accepts a Claude subscription OAuth token as its key and, like pi, sends Claude Code's headers and user agent with it (`newAnthropicClient` in `agent/clients.go`)
- **Extensions**: skills, prompt templates and packages shared over npm or git

## Implementation Priority

The first two steps of the plan this doc proposed are done: `agent/provider.go` exists and wraps the Anthropic SDK. What remains:

1. **Make the interface neutral**: move it off `anthropic.MessageNewParams` onto inber types
2. **Migrate gradually**: replace direct `anthropic.*` use with the interface
3. **Fold the OpenAI loop in**: make the OpenAI-compatible and Codex subscription clients implementations of that interface, and delete `runOpenAITurn`

## Bottom Line

Pi excels at **modularity and provider abstraction**. Inber excels at **server-side orchestration, limits and memory**.

The biggest lesson still holds: **abstract the LLM provider early**. Inber added OpenAI support in 2026 by writing a second turn loop and two format converters, not by fitting a new provider behind one interface, because its types were Anthropic's from the start.
