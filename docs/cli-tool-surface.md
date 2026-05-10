# Inber-Runtime CLI Surface

## Scope

This doc covers the `inber` CLI specifically as a client of inber-the-runtime — interactive chat, single-shot runs, inber-hosted session inspection, inber-runtime-specific config. **Cross-harness and cross-store operations** (memory, notes, bus, agent dispatch, tool-store invocation) **are not in scope here** — those live behind the `bridge` CLI documented in `~/repos/llm-bridge-server/CLI-SURFACE.md`.

The earlier framing of "inber CLI as universal tool surface" was a holdover from when inber was the orchestrator. Now that inber is one harness among many, only inber-runtime-specific operations belong on this binary.

## What inber-cli should keep

The following commands target inber-the-runtime and stay under `inber`:

| Subcommand | Purpose |
|---|---|
| `inber chat` | Interactive REPL session against inber's HTTP API |
| `inber run [message]` | Single-shot send to an inber agent, print response |
| `inber btw <session-key> <message>` | Inject a message into a running inber session |
| `inber sessions list/active/show/context/timeline/prompts/prompt` | Inspect inber-hosted sessions specifically |
| `inber agents show <name>` | Inspect inber-hosted agent runtime config |
| `inber config show/init/user` | Manage `~/.config/inber` config |
| `inber models list` | List models inber knows about (inber-runtime-specific config view) |

These all operate on inber's HTTP API and have nothing to do with cross-harness orchestration.

## What inber-cli should NOT expose

Listed in `~/repos/llm-bridge-server/CLI-SURFACE.md`. Briefly:

- `inber agent ask <slug>` — cross-harness delegation. Belongs on `bridge agent ask`.
- `inber memory ...` — memory-store is a service. Belongs on `bridge memory ...`.
- `inber notes ...` — noteboard is a service. Belongs on `bridge notes ...`.
- `inber bus ...` — bus is a service. Belongs on `bridge bus ...`.
- `inber tools list/run` — tool-store is a service. Belongs on `bridge tools ...`.
- `inber skills get` — skill-store is a service. Belongs on `bridge skills ...`.

## Output conventions (still apply within scope)

For the commands inber-cli does keep, the conventions from earlier still hold:

- `--json` everywhere, with `INBER_JSON=1` flipping the whole session.
- `--from-stdin` and `--input-file <path>` for any subcommand that takes free-form text.
- stderr format: single-line summary on first line, optional structured detail below.
- Exit non-zero on every error path.
- `inber <cmd> --help` is consistent and cheap.

These improve the developer experience whether the consumer is a human at a terminal or an agent driving inber via Bash.

## Acceptance checklist (inber-runtime scope only)

- [ ] `--json` works on every retained subcommand
- [ ] `INBER_JSON=1` flips the whole session
- [ ] `--from-stdin` / `--input-file` on any free-form-text subcommand
- [ ] stderr format standardized
- [ ] `inber <cmd> --help` consistent

The cross-harness CLI surface (everything previously described in this doc as "missing subcommands") is deferred to `bridge-cli`. See `~/repos/llm-bridge-server/CLI-SURFACE.md` for the design.
