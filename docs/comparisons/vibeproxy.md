# VibeProxy Comparison

**Project**: [VibeProxy](https://github.com/automazeio/vibeproxy)  
**Language**: Swift (macOS native)  
**Focus**: Proxy layer to reuse existing AI subscriptions (Claude Max, ChatGPT, Gemini, etc.) with coding tools — no API keys needed  
**Key Strengths**: One-click OAuth, multi-provider round-robin, native macOS menu bar UX

## What It Is

VibeProxy is a native macOS menu bar app that acts as an OpenAI-compatible proxy server. It lets you route requests from AI coding tools (like Factory Droids, Amp CLI) through your existing consumer subscriptions (Claude Code, ChatGPT/Codex, Gemini, Qwen, Z.AI GLM) instead of paying for separate API access.

Built on [CLIProxyAPIPlus](https://github.com/router-for-me/CLIProxyAPIPlus), it handles OAuth authentication, token management, and API routing automatically. It also supports Vercel AI Gateway integration for safer Claude Max access.

## Relationship to Inber

**Different layer entirely.** VibeProxy is an API proxy/router — it sits between a client and upstream AI providers, translating subscription-based access into API-compatible endpoints. Inber is a conversation engine and agent runtime.

They could theoretically be **complementary**: VibeProxy could serve as a provider backend that inber routes through, though inber already handles multi-provider routing natively via its engine configuration.

## Key Differences

| Aspect | VibeProxy | Inber |
|--------|-----------|-------|
| **Purpose** | Proxy subscription access → API endpoints | Conversation engine, agent runtime |
| **Scope** | Transport/auth layer only | Full prompt lifecycle (context, memory, tools, truncation) |
| **Multi-provider** | Round-robin + failover between accounts | Intelligent routing with model selection per-turn |
| **Platform** | macOS only (native Swift app) | Cross-platform (Go) |
| **Auth model** | OAuth token harvesting from browser sessions | Standard API keys / provider configs |
| **Output** | OpenAI-compatible API endpoint | Structured conversation sessions with tool use |

## What They Do Well

### 1. Zero-Friction Setup
One-click OAuth flow that detects credentials from browser sessions. No API key management needed for supported providers.

### 2. Multi-Account Round-Robin
Connect multiple accounts per provider with automatic failover when rate-limited — useful for heavy usage on subscription plans.

### 3. Vercel AI Gateway Integration
Routes Claude requests through Vercel's AI Gateway to reduce risk of account suspension from direct OAuth token usage.

## Not Relevant to Inber

- **Subscription arbitrage** — VibeProxy's core value prop (using consumer subscriptions as API) is orthogonal to inber's concerns
- **macOS-only** — inber targets server/CLI environments cross-platform
- **No conversation management** — VibeProxy is a dumb pipe; it doesn't manage context, memory, or tool orchestration
