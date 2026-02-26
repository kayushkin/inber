# Inber

An agent orchestration framework in Go. Named after Inber Scéne — the bay where the Milesians first landed in Ireland.

## Quick start

### Running from project directory
```bash
export ANTHROPIC_API_KEY=your-key
go run ./cmd/inber "your prompt"
```

### Installing for global use
```bash
go build -o inber ./cmd/inber/
sudo mv inber /usr/local/bin/  # or add to your PATH

# Set up user config (works from any directory)
inber config user
```

See [User Configuration Guide](docs/user-config.md) for details on config locations and priority.

## Test

```bash
export ANTHROPIC_API_KEY=your-key
go test ./...
```

## Architecture

- `agent/` — core agent loop (message → tool calls → response)
- `cmd/inber/` — CLI entrypoint

Built on [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go).
