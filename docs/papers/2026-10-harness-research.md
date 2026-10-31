# Harness Research — October 2026

Notes from the October harness-watch sweeps. Each paper here was checked against the
2026-04 to 2026-09 files and `docs/comparisons/agentic-design-patterns.md` by arXiv id, and
its title, date and numbers were read from `arxiv.org/abs/` and the paper's HTML, not a
search snippet.

# 2026-10-26 sweep

Two web searches. Of the ten in-scope ids they returned, two were new to these docs. One is
below. The other, MemPilot ([2610.06830](https://arxiv.org/abs/2610.06830), 2026-10-05),
trains a memory-curation policy with reinforcement learning. inber does not train
policies, so it is left out. No sweep ran between 2026-09-17 and today, so the paper below
is 39 days old, nine past the usual 30-day window, and is included because nothing else
caught it.

## 1. Stashing elided content for recall buys almost nothing

[arXiv:2609.20804](https://arxiv.org/abs/2609.20804) — **An Empirical Study of Harness
Design for Coding Agents** (2026-09-17). One fixed agent loop; planning, action space and
context management varied one at a time. 176 matched settings over four models, on
SWE-Bench Verified and Terminal-Bench 2.1, at windows of 32K to 128K.

- **Context management pays mostly by preventing overflow.** At 32K on SWE-Bench, managed
  runs beat no management by an average 35.7 points; at 128K the gap is 2.7.
- **Elision first, then summarize, gives the best accuracy for the cost.** Stale bulky tool
  outputs become stubs once history passes 0.6 of the window; an LLM summary of the middle
  runs only past 0.85. The preamble and the recent turns stay verbatim.
- **Making elided content recoverable did nothing.** In 36 of the 64 settings that offered
  `recall_event`, the agent never called it, and the median call rate was zero. Mean calls
  per task fell from 0.540 at 32K to 0.007 at 128K. Accuracy did not improve.
- **Bash-only can be cheaper for strong models and is ruinous for weak ones.** On SWE-Bench
  the 550B model went from 65.8% at $2.33 per task with predefined tools to 69.4% at $1.11
  with bash only. The 30B model fell from 25.2% to 10.2%.

**What inber should consider:** `ManageConversation` stashes large blocks into memory-store
before it prunes anything (`conversation/manage.go:66-72`). Each stash leaves a stub telling
the model to call `memory_expand` or `memory_search` (`conversation/stash.go:70-92`, `:207-209`).
That is the paper's T2/T4 recall arm, and inber pays a memory-store write for every stash.
Before building more on stashing, count how often a session calls `memory_expand` on a
stashed id compared with how many stashes it wrote. If the ratio is near the paper's, a stub
with no write would cost less and lose nothing measurable. The local
`~/.inber/memory.db` was last written 2026-03-25 and cannot answer this; the count has
to come from memory-store and the session event log.

# 2026-10-28 sweep

Two web searches. One in-scope id was new to these docs.

## 2. LLM summaries keep at most 16% of the details a task keeps needing

[arXiv:2610.31710](https://arxiv.org/abs/2610.31710) — **Compact-Bench: Measuring What Context
Compaction Costs Long-Horizon Coding Agents** (2026-10-22). Each case resumes the same repository
state twice, once with full history and once from a summary, over 35 SWE-bench repairs, 146 agent
sessions and 106 production chains.

- Across 14 summarizer models, compaction broke 5% to 64% of the 22 repairs that full history solved.
- LLM summaries kept at most 16.0% of *recurring details* (things the later turns use again);
  mechanical extraction kept 79.1%. Production summaries kept 15.7% at the first compaction.
- The prompt matters as much as the model: with one summarizer, the five prompts tested ranged
  widely, and one repair was broken by 13 of 14 models.
- Keeping the relevant facts did not by itself let the agent finish; the summary has to carry
  progress and the next step.

**What inber should consider:** `SummarizeConversation` exchanges the older turns for one LLM
summary written by the session's own model under a generic prompt (topics, decisions, status, next
steps; `conversation/summary_generation.go:40-55`), capped at 800–1024 tokens
(`conversation/summarize_config.go:31-55`). Nothing mechanical rides alongside it. A cheap
experiment: before summarizing, pull the file paths, ids, commands and error strings that occur in
two or more of the dropped turns, and append them verbatim under the summary. Score it by the
paper's own test — does the resumed session finish — not by how the summary reads.

## 3. Rolling drop of old tool responses beats threshold summarization, on a serving stack that can drop without re-prefill

[arXiv:2610.28577](https://arxiv.org/abs/2610.28577) — **Cache Engineering for Agents**
(2026-10-20; code at github.com/thu-nics/AKV). Trimming or summarizing history changes the prefix,
so every later cached state is lost and the kept context has to be prefilled again. The paper lets
the agent `drop` chosen KV entries from future attention and `repos` the kept keys' positions,
without rewriting the token sequence. Its AKV serving engine keeps a stateless API and tracks the
cache state, because the same tokens can now map to different caches. A policy that drops tool
responses as they age scored 1.7 points higher on average across three models and four benchmarks
than threshold-triggered summarization, and cost 27.7% less. Old tokens and tool responses drew less
attention than recent turns and assistant text.

**What inber should consider:** inber calls hosted APIs, so it cannot drop KV entries; its age-based
pass (`conversation/manage_tool_pruning.go:30-56`: keep 3 turns, summarize, drop after 8 in the
default config at `conversation/manage_config.go:48-50`) rewrites the prefix and pays the re-prefill
the paper avoids. The transferable part is the ordering: the paper's accuracy result supports aging
tool results out before summarizing anything. The open question for inber is cost, not accuracy:
each age step moves the rewrite point and breaks the cache from there. Moving the age boundary
in steps (drop eight turns' results at once, not one per turn) would trade a little context for
fewer cache breaks; measure cache-read share per session before and after.

# 2026-10-30 sweep

Two web searches. Two in-scope ids were new to these docs. One is below. The other, AutoCompact
([2610.02163](https://arxiv.org/abs/2610.02163), 2026-10-01), trains the agent with SFT then RL to choose
when to compact (+9.2 points on SWE-bench Verified over its base model). inber does not train policies, so
it is left out.

## 4. Compaction that only cuts, and never compacts a compaction

[arXiv:2609.26779](https://arxiv.org/abs/2609.26779) — **CliffCompaction: Cost-Efficient Compaction for
Long-Horizon Coding Agents** (2026-09-22; 38 days old, included because no sweep caught it). Context grows
to a threshold, then the method truncates or drops content and never rephrases it. Each pass works from the
original content and throws the last pass's output away, so no compaction is ever compacted again. The
abstract reports cost down by up to 50% under a bounded context, Terminal-Bench held or improved, and over
10 points more under parallel test-time scaling for less than the cost of two full-context runs. Numbers
read from the abstract only.

**What inber should consider:** inber's tool-result pass already works this way: it cuts mechanically and
keeps the tool name and size in the marker, and the `tool_use` block with its inputs stays, so the agent
can call again (`conversation/manage_tool_pruning.go:39-56`, `142-147`). `SummarizeConversation` does not.
It rephrases, and on the second compaction the earlier summary block sits at index 0 of `oldMessages`
(`conversation/summarize.go:47`, `112-120`), so the new summary is written from the old one. Together with
item 2 (LLM summaries keep at most 16% of recurring details), the test worth running is: keep each
compaction's source turns in the archive and write every new summary from those originals, not from the
last summary, and compare finish rates on resumed sessions.
