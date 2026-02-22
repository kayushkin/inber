# Inber

An agent orchestration framework in Go. Named after Inber Scéne — the bay where the Milesians first landed in Ireland.

## Quick start

```bash
export ANTHROPIC_API_KEY=your-key
go run ./cmd/inber
```

## Test

```bash
export ANTHROPIC_API_KEY=your-key
go test ./...
```

## Architecture

- `agent/` — core agent loop (message → tool calls → response)
- `cmd/inber/` — CLI entrypoint

Built on [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go).
