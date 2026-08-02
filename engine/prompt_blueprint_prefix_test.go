package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	sessionMod "github.com/kayushkin/inber/session"
)

// The blueprint exists to answer one question: did this turn's request keep the
// prefix the last one cached? Anthropic hashes tools -> system -> messages as an
// ordered byte sequence, so the answer depends on position and content and on
// nothing else. These tests pin that, in both directions: the diff must not
// report a hit the request did not get, and must not report a miss it did not
// suffer.

// memoryBlockLabel reproduces the label engine/turn_prompt.go builds for a
// memory's system block, so these tests break if that format starts carrying
// something the diff would trip over.
func memoryBlockLabel(id string, importance float64, tags ...string) string {
	label := fmt.Sprintf("%s (%.1f", id, importance)
	if len(tags) > 0 {
		label += ", tags: " + strings.Join(tags, ",")
	}
	return label + ")"
}

// systemBlueprint builds a blueprint holding one system section, with the
// breakpoint on the last block exactly as buildSystemBlocks places it.
func systemBlueprint(turn int, labels, texts []string) *PromptBlueprint {
	if len(labels) != len(texts) {
		panic("labels and texts must be the same length")
	}
	named := make([]sessionMod.NamedBlock, 0, len(labels))
	system := make([]anthropic.TextBlockParam, 0, len(texts))
	for i := range labels {
		named = append(named, sessionMod.NamedBlock{ID: labels[i], Text: texts[i]})
		system = append(system, anthropic.TextBlockParam{Text: texts[i]})
	}
	if len(system) > 0 {
		system[len(system)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return BuildBlueprint(turn, nil, system, named, nil, 0)
}

// statusByPosition flattens a diff to one status per block, in request order.
func statusByPosition(d *BlueprintDiff) []string {
	var out []string
	for _, s := range d.Sections {
		for _, b := range s.Blocks {
			out = append(out, b.Status)
		}
	}
	return out
}

func breakpointStatus(t *testing.T, d *BlueprintDiff) string {
	t.Helper()
	for _, s := range d.Sections {
		for _, b := range s.Blocks {
			if b.Cache != "" {
				return b.Status
			}
		}
	}
	t.Fatalf("diff carried no breakpoint block")
	return ""
}

// Two memories swapping places changes the system array's bytes, so the cached
// prefix is gone. Keyed by label, every block still matches something in the
// previous turn and the diff calls the whole prefix stable.
func TestDiffSeesAReorderedSystemPrefixAsBusted(t *testing.T) {
	first := memoryBlockLabel("aaaaaaaa", 0.4, "repo")
	second := memoryBlockLabel("bbbbbbbb", 0.4, "repo")
	tail := memoryBlockLabel("cccccccc", 0.9)

	prev := systemBlueprint(1, []string{first, second, tail}, []string{"alpha", "beta", "gamma"})
	curr := systemBlueprint(2, []string{second, first, tail}, []string{"beta", "alpha", "gamma"})

	d := DiffBlueprints(prev, curr)
	if got := breakpointStatus(t, d); got == "HIT" {
		t.Fatalf("reordered system blocks reported the breakpoint as %q; the request's bytes changed, so it cannot hit", got)
	}
	if d.Summary.CachedRead != 0 {
		t.Fatalf("reordered prefix reported %d tokens read from cache, want 0", d.Summary.CachedRead)
	}
}

// Dropping a memory shortens the system array, which busts the prefix too — and
// the surviving blocks keep their labels, so a label-keyed diff sees only
// matches and never mentions the block that left.
func TestDiffSeesARemovedSystemBlockAsBusted(t *testing.T) {
	first := memoryBlockLabel("aaaaaaaa", 0.4)
	middle := memoryBlockLabel("bbbbbbbb", 0.4)
	tail := memoryBlockLabel("cccccccc", 0.9)

	prev := systemBlueprint(1, []string{first, middle, tail}, []string{"alpha", "beta", "gamma"})
	curr := systemBlueprint(2, []string{first, tail}, []string{"alpha", "gamma"})

	d := DiffBlueprints(prev, curr)
	if got := breakpointStatus(t, d); got == "HIT" {
		t.Fatalf("a dropped system block reported the breakpoint as %q; the request's bytes changed, so it cannot hit", got)
	}

	var removed int
	for _, s := range d.Sections {
		for _, b := range s.Blocks {
			if b.Status == "REMOVED" {
				removed++
			}
		}
	}
	if removed != 1 {
		t.Fatalf("diff listed %d REMOVED blocks, want 1 — a block that left the request has to be visible", removed)
	}
}

// A memory dropping off the END of the system section is the case position
// alone cannot see: every block still sent matches the block that sat at its
// offset last turn, and the messages after it match too. What moved is where
// the messages START, so their breakpoints cannot hit — and nothing downstream
// of the shortened section would notice on its own.
func TestDiffSeesATrailingRemovalMovingEverythingAfterIt(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "ask"}}}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "answer"}}}},
	}
	build := func(turn int, texts []string) *PromptBlueprint {
		named := make([]sessionMod.NamedBlock, 0, len(texts))
		system := make([]anthropic.TextBlockParam, 0, len(texts))
		for i, text := range texts {
			named = append(named, sessionMod.NamedBlock{ID: memoryBlockLabel(fmt.Sprintf("mem%05d", i), 0.4), Text: text})
			system = append(system, anthropic.TextBlockParam{Text: text})
		}
		system[len(system)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
		return BuildBlueprint(turn, nil, system, named, messages, len(messages)-1)
	}

	prev := build(1, []string{"alpha", "beta", "gamma"})
	curr := build(2, []string{"alpha", "beta"})

	d := DiffBlueprints(prev, curr)

	var messageBreakpoints int
	for _, s := range d.Sections {
		if s.Name != "messages" {
			continue
		}
		for _, b := range s.Blocks {
			if b.Cache == "" {
				continue
			}
			messageBreakpoints++
			if b.Status == "HIT" {
				t.Fatalf("%s reported HIT after the system section lost a block; every message moved, so it cannot hit", b.ID)
			}
		}
	}
	if messageBreakpoints == 0 {
		t.Fatalf("test is inert: the messages section carried no breakpoint to check")
	}
}

// The other direction. A memory's importance drifts on every read, and the label
// carries it at one decimal place, so a block whose text never moved gets a new
// label and is reported as a brand-new block cascading over everything after it.
func TestDiffIgnoresARelabelledBlockWhoseTextIsIdentical(t *testing.T) {
	before := memoryBlockLabel("aaaaaaaa", 0.4, "repo")
	after := memoryBlockLabel("aaaaaaaa", 0.5, "repo")
	if before == after {
		t.Fatalf("test is inert: both labels are %q", before)
	}
	tail := memoryBlockLabel("cccccccc", 0.9)

	prev := systemBlueprint(1, []string{before, tail}, []string{"alpha", "gamma"})
	curr := systemBlueprint(2, []string{after, tail}, []string{"alpha", "gamma"})

	d := DiffBlueprints(prev, curr)
	if got := breakpointStatus(t, d); got != "HIT" {
		t.Fatalf("breakpoint reported %q after a label-only change; the system bytes are identical, so it hits", got)
	}
	for _, status := range statusByPosition(d) {
		if status != "STABLE" && status != "HIT" {
			t.Fatalf("statuses %v: a label-only change must leave every block stable", statusByPosition(d))
		}
	}
}

// Control for the two above: real text changes must still be caught, or the fix
// could be to report everything stable.
func TestDiffStillReportsChangedTextAsBusted(t *testing.T) {
	head := memoryBlockLabel("aaaaaaaa", 0.4)
	tail := memoryBlockLabel("cccccccc", 0.9)

	prev := systemBlueprint(1, []string{head, tail}, []string{"alpha", "gamma"})
	curr := systemBlueprint(2, []string{head, tail}, []string{"alpha rewritten", "gamma"})

	d := DiffBlueprints(prev, curr)
	if got := statusByPosition(d)[0]; got != "CHANGED" {
		t.Fatalf("rewritten block reported as %q, want CHANGED", got)
	}
	if got := breakpointStatus(t, d); got != "WRITE" {
		t.Fatalf("breakpoint after a rewritten block reported %q, want WRITE", got)
	}
}

// Control: an untouched turn hits, or the fix could be to report everything busted.
func TestDiffStillReportsAnUntouchedPrefixAsAHit(t *testing.T) {
	head := memoryBlockLabel("aaaaaaaa", 0.4)
	tail := memoryBlockLabel("cccccccc", 0.9)

	prev := systemBlueprint(1, []string{head, tail}, []string{"alpha", "gamma"})
	curr := systemBlueprint(2, []string{head, tail}, []string{"alpha", "gamma"})

	d := DiffBlueprints(prev, curr)
	if got := breakpointStatus(t, d); got != "HIT" {
		t.Fatalf("identical turn reported the breakpoint as %q, want HIT", got)
	}
}

// The summary is the number a reader acts on. Every token in the request is
// read, written or uncached — exactly one of the three — so the three must add
// up to the total in every scenario.
func TestDiffSummaryTokensAddUpToTheTotal(t *testing.T) {
	head := memoryBlockLabel("aaaaaaaa", 0.4)
	tail := memoryBlockLabel("cccccccc", 0.9)
	prev := systemBlueprint(1, []string{head, tail}, []string{"alpha", "gamma"})

	cases := []struct {
		name string
		curr *PromptBlueprint
	}{
		{"identical", systemBlueprint(2, []string{head, tail}, []string{"alpha", "gamma"})},
		{"head rewritten", systemBlueprint(2, []string{head, tail}, []string{"alpha rewritten", "gamma"})},
		{"block appended", systemBlueprint(2, []string{head, tail, "sys:d"}, []string{"alpha", "gamma", "delta"})},
		{"block removed", systemBlueprint(2, []string{head}, []string{"alpha"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := DiffBlueprints(prev, tc.curr)
			s := d.Summary
			if sum := s.CachedRead + s.CachedWrite + s.Uncached; sum != s.TotalTokens {
				t.Fatalf("read %d + write %d + uncached %d = %d, want the total %d",
					s.CachedRead, s.CachedWrite, s.Uncached, sum, s.TotalTokens)
			}
		})
	}
}

// DiffBlueprints' own doc comment says blocks after the last breakpoint are
// always uncached. Nothing enforced it, and the summary counted a stable one as
// a cache read.
func TestBlocksAfterTheLastBreakpointAreUncached(t *testing.T) {
	named := []sessionMod.NamedBlock{
		{ID: "sys:head", Text: "alpha"},
		{ID: "sys:tail", Text: "gamma"},
	}
	system := []anthropic.TextBlockParam{
		{Text: "alpha", CacheControl: anthropic.NewCacheControlEphemeralParam()},
		{Text: "gamma"},
	}
	prev := BuildBlueprint(1, nil, system, named, nil, 0)
	curr := BuildBlueprint(2, nil, system, named, nil, 0)

	d := DiffBlueprints(prev, curr)
	tailTokens := estimateTokensStr("gamma")
	if d.Summary.Uncached != tailTokens {
		t.Fatalf("uncached = %d, want %d: the block after the last breakpoint is never cached",
			d.Summary.Uncached, tailTokens)
	}
	if d.Summary.CachedRead != estimateTokensStr("alpha") {
		t.Fatalf("cached read = %d, want %d: only the breakpoint's own prefix is read back",
			d.Summary.CachedRead, estimateTokensStr("alpha"))
	}
}
