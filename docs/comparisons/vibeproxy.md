# VibeProxy Comparison

**Last reviewed**: 2026-10-29 against automazeio/vibeproxy@1952266 (2026-10-29).

**Project**: [VibeProxy](https://github.com/automazeio/vibeproxy) (MIT, active)  
**Language**: Swift (macOS native)  
**Focus**: Proxy layer to reuse existing AI subscriptions (Claude Max, ChatGPT, Gemini, etc.) with coding tools — no API keys needed  
**Key Strengths**: One-click OAuth, multi-provider round-robin, native macOS menu bar UX

## What It Is

VibeProxy is a native macOS menu bar app that acts as an OpenAI-compatible proxy server. It lets you route requests from AI coding tools (like Factory Droids, Amp CLI) through your existing consumer subscriptions (Claude Code, ChatGPT/Codex, Gemini, Kimi, Qwen, Antigravity, Z.AI GLM, GitHub Copilot) instead of paying for separate API access.

It is built on [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) (checked at router-for-me/CLIProxyAPI@90c447d, 2026-10-28), which handles OAuth sign-in, token refresh and API routing. Earlier versions of this doc named CLIProxyAPIPlus; that fork has been retired and its repo no longer resolves. VibeProxy also supports Vercel AI Gateway for Claude Max access that does not use the OAuth token directly, and since v1.6 updates itself daily through Sparkle.

## Relationship to Inber

**Different layer.** VibeProxy is an API proxy/router — it sits between a client and upstream AI providers, turning subscription access into API-compatible endpoints. Inber is a conversation engine and agent runtime.

They could be **complementary**: VibeProxy could serve as a provider backend that inber routes through. Inber's OpenAI-compatible client fixes its base URL per provider name (`defaultBaseURL` in `agent/clients.go`), so pointing it at a local proxy would need a small code change. And inber now reaches subscriptions on its own, without a proxy (below).

## Key Differences

| Aspect | VibeProxy | Inber |
|--------|-----------|-------|
| **Purpose** | Proxy subscription access → API endpoints | Conversation engine, agent runtime |
| **Scope** | Transport/auth layer only | Full prompt lifecycle (context, memory, tools, truncation) |
| **Multi-provider** | Round-robin + failover between accounts | Model selection per turn with failover by model health (`engine/failover.go`) |
| **Platform** | macOS only (native Swift app) | Cross-platform (Go) |
| **Auth model** | OAuth sign-in through the browser for each provider; tokens kept and refreshed by the app | For Anthropic, an API key, or a Claude subscription OAuth token (`sk-ant-oat01-…`), which inber sends with Claude Code's headers and user agent (`newAnthropicClient` in `agent/clients.go`). For OpenAI models, either an API key or the ChatGPT subscription: when `OPENAI_CODEX_SUBSCRIPTION_AUTH_FILE` is set, inber reads the Codex CLI's own auth file and posts to the endpoint the Codex CLI uses, `chatgpt.com/backend-api/codex/responses` (`agent/codex_subscription.go`, `cmd/inber-server/settings.go`). Inber does no OAuth itself; the Codex CLI owns sign-in and refresh |
| **Output** | OpenAI-compatible API endpoint | Structured conversation sessions with tool use |

## What They Do Well

### 1. Zero-Friction Setup
One-click OAuth flow per provider in the browser; the app picks up the credentials. No API key management for supported providers.

### 2. Multi-Account Round-Robin
Connect several accounts per provider with automatic failover when rate-limited — useful for heavy use of subscription plans.

### 3. Vercel AI Gateway Integration
Routes Claude requests through Vercel's AI Gateway to reduce the risk of account suspension from direct OAuth token use.

## Relevance to Inber

- **Subscription access** — this doc first called VibeProxy's core idea, using consumer subscriptions as an API, unrelated to inber. That stopped being true in 2026: inber now serves OpenAI models through the ChatGPT subscription, the same trick CLIProxyAPI does for Codex. Inber's code comment notes the risk VibeProxy's users carry too: the endpoint is undocumented and can change or refuse non-Codex callers without notice, so the API-key path stays one setting away. Inber's Anthropic client also accepts a Claude subscription OAuth token and presents itself as Claude Code when given one (`agent/clients.go`); Anthropic's terms reserve subscription OAuth for its own apps and since 2026-04-04 bill third-party use as extra usage. Which credential a given install uses is set by its key, not by the code.
- **What VibeProxy has that inber lacks**: more than one account per provider, round-robin and failover across them, and its own OAuth sign-in and refresh. Inber obtains no token itself: it is handed one, or reads the Codex CLI's file.
- **macOS-only** — inber targets servers, so the app itself is no use; CLIProxyAPI, the Go server under it, is cross-platform.
- **No conversation management** — VibeProxy is a plain pipe; it does not manage context, memory, or tool orchestration.
