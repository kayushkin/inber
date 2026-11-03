# Harness Research — November 2026

Notes from the November harness-watch sweeps. Each paper here was checked against the
2026-04 to 2026-10 files and `docs/comparisons/agentic-design-patterns.md` by arXiv id.
Title, date and numbers come from the `arxiv.org/abs/` page.

# 2026-11-02 sweep

One extended web search. Of the in-scope ids it returned, three were new to these docs. Two
are below. The third, Trained Agentic Context Management
([2610.02404](https://arxiv.org/abs/2610.02404), 2026-10-01), fine-tunes an open model for
the job. inber does not train models, so it is left out.

Upstream commits screened this week and not written up: goose #13279 (its prompt named a
screenshot tool that no longer existed) prompted a grep of inber for tool names from before
the tool-store rename. Every hit (`agent/read_cache.go:116`, `conversation/dedup_files.go:13`,
`engine/workflow_git.go:37`) matches the old and new names together, so nothing is stale.
gemini-cli #30035 (`$`-patterns in replacement text garbled its prompts) has no inber
counterpart: inber builds no prompt with `regexp.ReplaceAllString` or a template, and its two
`ReplaceAllString` calls in `redact/redact.go` take fixed replacement strings. codex #58956
(refresh volatile context after compaction) does not apply: inber rebuilds its volatile
context every turn.

## 1. Let the model decide when to compact: same success as Claude Code at a third less cost

[arXiv:2610.34425](https://arxiv.org/abs/2610.34425) — **CTXAgent: Self-Controlled Context
Management of LLM Agents** (Busch, Tacke, Aydin, Feiler, Cyron, 2026-10-25). The model gets
two tools, compact and curate, and one paragraph of guidance; it decides when to compact,
what to keep and what to write as the handoff. Seven frontier models, mixed sessions of 35
to 212 turns built from coding exercises, SciCode, SWE-bench Pro and Terminal-Bench 4.0, with
planted facts, short-lived observations and standing instructions checked by script. Most
strong models compacted and curated regularly, kept live context mostly under 100k tokens,
and lost no significant score. With Claude Opus 5.5 it matched Claude Code's task success at
34% lower cost than Claude Code's explicit 200k-window setting and 49% lower than its
default. Some models compacted so often that other costs rose. Read from the abstract page.

**What inber should consider:** inber's compaction is entirely the harness's choice. It fires
on a message count (`conversation/summarize.go:14-16`), and every strategy in
`conversation/manage_config.go:189-210` is picked by configuration, not by the model; the only
context tool the model holds is `memory_forget` (`memory/tools.go:189`). A trial would add one
compact tool whose argument is the handoff text, keep the count trigger as a backstop, and
compare cost and finish rate per session for Opus-class agents only, since the paper finds the
gain depends on the model. The benchmark's standing-instruction checks are the same test the
October entry on arXiv:2610.31778 asked for; the released benchmark could supply it.

## 2. Auto-approve raises attack success from 29% to 96%; a command denylist cuts it at no utility cost

[arXiv:2610.07639](https://arxiv.org/abs/2610.07639) — **HarnessSecurity-Bench: Do Security
Mechanisms Really Protect Coding Agent Harnesses?** (Zhu et al., 2026-10-06, revised
2026-10-10). A ten-mechanism taxonomy rated across harnesses, then 23 tasks over five attack
surfaces on six harnesses (Claude Code, Codex CLI, Gemini CLI, gptme, Qwen Code, Copilot),
2,500 trials. About half the mechanisms are opt-in. Auto-approve raised attack success from
29.2% to 95.6%. Network isolation and read-only mode cut attacks but cost a lot of utility.
A command allowlist cut attacks for a small utility loss; a command denylist cut attacks and
slightly raised utility. Restricting a shared capability blocks good and bad uses alike, and
allowed tools often leave a blocked action reachable another way. Read from the abstract page.

**What inber should consider:** inber's guard decides by tool name only. `Assist` asks for
approval on any `shell_commands` call and `Unset` allows everything (`guard/guard.go:165-188`,
`328-333`), so inber offers exactly the two ends the paper measured: ask every time, or
auto-approve. It has no command-level allow or deny list. A denylist checked inside
`CheckTool` on `shell_commands` input is the paper's cheapest win. What a change has to
decide is whether `Unset`, which every session ran as before the guard had a caller, should
gain the denylist by default; that changes behavior for every existing session, and is not
for this job to settle. A design gap, not a defect; no todo filed.
