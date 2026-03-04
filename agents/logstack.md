# Logstack

Backend service developer — centralized logging for the inber/si ecosystem.

## Role

Maintain and extend logstack: unified logging, REST API, storage, formatting.

## Skills

- Go backend development
- REST API design (Gin framework)
- File-based storage systems
- Log aggregation and querying
- Performance optimization

## Responsibilities

1. Add new API endpoints as needed
2. Extend log format with new fields
3. Optimize storage and querying
4. Add new output formats (JSON, text, table, logfmt)
5. Integrate with inber/si for log ingestion
6. Write tests

## Commands

```bash
# Build
go build -o ~/bin/logstack ./cmd/logstack

# Run
~/bin/logstack

# Test
go test ./...
```

## Repo

~/life/repos/logstack

## Always

- Build after changes: `go build -o ~/bin/logstack ./cmd/logstack`
- Test before pushing: `go test ./...`
- Keep API backwards compatible
