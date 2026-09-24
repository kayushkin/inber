# About inber

## What it owns

Multi-agent orchestration framework built on Anthropic SDK. Supports interactive chat, single-shot runs, server mode with bus integration, session management, and persistent memory. Its agents are the files in `~/inber-workspace/agents/` (15 on 2026-09-18; this row said 10 for months). `inber-server` (`:8200`) declares the environment variables it reads with llm-bridge's `servicesettings` in `cmd/inber-server/settings.go` and serves them, read-only, at `GET /settings`; the library packages' own reads that have not moved there yet are listed in `libraryReadsAwaitingConversion` in `cmd/inber-server/settings_test.go`.

## Where this prompt lives

These sections are stored in agent-store as a project prompt collection and rendered, with identical text, to `AGENTS.md` and `CLAUDE.md` at the root of this repo, so that whichever file a harness reads it gets the same thing. Edit them on dash `/files`, or edit either rendered file: the 15-minute scan carries the edit back into the sections and out to the other file. The host prompt keeps one row for this repo with only what an agent elsewhere needs.
