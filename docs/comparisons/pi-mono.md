# Pi-Mono Comparison

**Project**: [pi-mono](https://github.com/badlogic/pi-mono) by Mario Zechner  
**Language**: TypeScript/Node.js  
**Focus**: Modular AI agent toolkit with clean provider abstraction  

## Architecture Overview

Pi-mono uses a clean monorepo structure with well-separated packages:

- `@mariozechner/pi-ai` — Unified multi-provider LLM API
- `@mariozechner/pi-agent-core` — Agent runtime with tool calling
- `@mariozechner/pi-coding-agent` — CLI agent implementation
- `@mariozechner/pi-tui` — Terminal UI components
- `@mariozechner/pi-web-ui` — Web interface components
- `@mariozechner/pi-mom` — Slack bot integration
- `@mariozechner/pi-pods` — vLLM deployment management

## What They Do Well

### 1. **Clean Provider Abstraction**
The `pi-ai` package provides a registry-based system for LLM providers:

```typescript
interface ApiProvider<TApi extends Api = Api> {
  api: TApi;
  stream: StreamFunction<TApi, TOptions>;
  streamSimple: StreamFunction<TApi, SimpleStreamOptions>;
}

function registerApiProvider<TApi extends Api>(provider: ApiProvider<TApi>): void;
function getApiProvider(api: Api): ApiProviderInternal | undefined;
```

This allows swapping providers without touching agent logic. Supports OpenAI, Anthropic, Google Gemini, Bedrock, Mistral, and many others.

### 2. **Package Modularity**
Each package has a clear, single responsibility:
- **pi-ai**: Pure LLM abstraction, no agent logic
- **pi-agent**: Pure agent logic, no provider coupling
- **pi-coding-agent**: CLI application layer

The agent package imports only the abstract types from pi-ai, never provider-specific SDKs.

### 3. **OAuth & Authentication Abstraction**
Unified OAuth flows for different providers with automatic token management. Each provider gets its own auth strategy but exposed through common interfaces.

### 4. **Tool System**
Tools are self-contained and provider-agnostic. The agent core handles tool orchestration without knowing about LLM specifics.

## What Inber Could Adopt

### 1. **Provider Interface Pattern**
Inber should extract a thin `Provider` interface similar to pi-mono's `ApiProvider`:

```go
type Provider interface {
    Complete(ctx context.Context, messages []Message, tools []Tool, options *Options) (*Response, error)
    Stream(ctx context.Context, messages []Message, tools []Tool, options *Options) (<-chan StreamEvent, error)
}
```

This would replace direct Anthropic SDK usage (currently 372+ references) with a pluggable system.

### 2. **Package Separation**
Consider splitting inber into modules:
- `inber/provider` — LLM provider abstraction
- `inber/agent` — Provider-agnostic agent runtime  
- `inber/engine` — Application orchestration
- `inber/cmd` — CLI commands

### 3. **Registry Pattern**
Use a registry for providers instead of hardcoded clients:

```go
type ProviderRegistry struct {
    providers map[string]Provider
}

func (r *ProviderRegistry) Register(name string, p Provider)
func (r *ProviderRegistry) Get(name string) Provider
```

### 4. **Unified Message Types**
Define inber-native message types that providers convert to their native formats, rather than exposing `anthropic.Message` throughout the codebase.

## Key Differences

### Inber Advantages
- **Go ecosystem**: Better for system tools, faster startup, easier deployment
- **Memory system**: More sophisticated conversation memory and summarization
- **Workspace isolation**: Better file system sandboxing and git integration
- **CLI focus**: More mature terminal-based workflow

### Pi-mono Advantages  
- **Provider flexibility**: Can easily swap between 15+ LLM providers
- **Web-first**: Better browser integration and UI components
- **Modular design**: Cleaner package boundaries and separation of concerns
- **OAuth infrastructure**: More sophisticated auth handling

## Implementation Priority

If implementing provider abstraction in inber:

1. **Start small**: Create `agent/provider.go` with minimal interface
2. **Wrap existing**: Make Anthropic SDK the first implementation
3. **Migrate gradually**: Replace direct `anthropic.*` usage with provider interface
4. **Add providers**: Implement OpenAI, then others as needed

The goal isn't to match pi-mono's 15+ providers immediately, but to remove the tight coupling that makes provider switching impossible.

## Bottom Line

Pi-mono excels at **modularity and provider abstraction**. Inber excels at **system integration and memory management**. 

Inber should adopt pi-mono's clean separation between LLM providers and agent logic, while keeping its strengths in terminal workflow and memory systems.

The biggest lesson: **Abstract the LLM provider early**. It's much harder to retrofit this abstraction (as inber's 372 anthropic references show) than to design it from the start.