package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
)

// SaveBlueprintToWorkspace persists the blueprint as JSON for cross-invocation diffs.
func SaveBlueprintToWorkspace(ws *sessionMod.Workspace, bp *PromptBlueprint) {
	path := filepath.Join(ws.Dir, "last_blueprint.json")
	data, err := json.Marshal(bp)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
}

// LoadBlueprintFromWorkspace loads the previous blueprint for diffing.
func LoadBlueprintFromWorkspace(ws *sessionMod.Workspace) (*PromptBlueprint, error) {
	path := filepath.Join(ws.Dir, "last_blueprint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bp PromptBlueprint
	if err := json.Unmarshal(data, &bp); err != nil {
		return nil, err
	}
	return &bp, nil
}

// PromptBlueprint is a structural manifest of a prompt request, showing each
// block's hash, size, cache control state, and predicted cache behavior.
// Compare blueprints across turns to see exactly what changed and what
// will hit/miss cache.
type PromptBlueprint struct {
	Turn     int                `json:"turn"`
	Sections []BlueprintSection `json:"sections"`
}

// BlueprintSection groups blocks by their role in the request.
type BlueprintSection struct {
	Name   string           `json:"name"`
	Blocks []BlueprintBlock `json:"blocks"`
}

// BlueprintBlock represents one content block in the prompt with its hash
// and cache metadata.
//
// A block's identity is its POSITION in the request, not its ID. Anthropic
// hashes tools -> system -> messages as one ordered byte sequence, so two turns
// carry the same block only when the same bytes sit at the same offset. The ID
// is a label for the reader: a memory's label embeds its importance, which
// drifts on every read, and two memories can swap places without either label
// changing. DiffBlueprints compares by position for exactly that reason.
type BlueprintBlock struct {
	ID     string `json:"id"`               // display label (tool name, memory description, msg role+index)
	Hash   string `json:"hash"`             // first 8 chars of SHA-256
	Tokens int    `json:"tokens"`           // estimated token count
	Cache  string `json:"cache,omitempty"`  // "BP1"/"BP2"/"BP3" if breakpoint, empty otherwise
	Status string `json:"status,omitempty"` // "HIT"/"MISS"/"WRITE"/"UNCACHED" (set by diff)
}

// BlueprintDiff compares two blueprints and annotates cache behavior.
type BlueprintDiff struct {
	Turn     int           `json:"turn"`
	PrevTurn int           `json:"prev_turn"`
	Sections []DiffSection `json:"sections"`
	Summary  DiffSummary   `json:"summary"`
}

type DiffSection struct {
	Name   string      `json:"name"`
	Blocks []DiffBlock `json:"blocks"`
}

type DiffBlock struct {
	ID       string `json:"id"`
	Hash     string `json:"hash"`
	Tokens   int    `json:"tokens"`
	Cache    string `json:"cache,omitempty"`
	Status   string `json:"status"`              // HIT, MISS, WRITE, NEW, REMOVED, UNCACHED
	PrevHash string `json:"prev_hash,omitempty"` // if changed
}

type DiffSummary struct {
	TotalTokens   int `json:"total_tokens"`
	CachedRead    int `json:"cached_read"`  // tokens predicted to hit cache
	CachedWrite   int `json:"cached_write"` // tokens predicted to be written to cache
	Uncached      int `json:"uncached"`     // tokens with no cache control
	BlocksChanged int `json:"blocks_changed"`
	BlocksStable  int `json:"blocks_stable"`
}

// BuildBlueprint creates a structural manifest of the current prompt.
func BuildBlueprint(
	turn int,
	tools []agent.Tool,
	systemBlocks []anthropic.TextBlockParam,
	namedBlocks []sessionMod.NamedBlock,
	messages []anthropic.MessageParam,
	frozenIdx int,
) *PromptBlueprint {
	bp := &PromptBlueprint{Turn: turn}

	// Tools section (evaluated first in Anthropic prefix)
	var toolBlocks []BlueprintBlock
	for i, t := range tools {
		schemaBytes, _ := json.Marshal(t.InputSchema)
		content := t.Name + t.Description + string(schemaBytes)
		h := shortHash(content)
		tokens := estimateTokensStr(content)
		cache := ""
		if i == len(tools)-1 {
			cache = "BP1"
		}
		toolBlocks = append(toolBlocks, BlueprintBlock{
			ID: fmt.Sprintf("tool:%s", t.Name), Hash: h, Tokens: tokens, Cache: cache,
		})
	}
	bp.Sections = append(bp.Sections, BlueprintSection{Name: "tools", Blocks: toolBlocks})

	// System section — use namedBlocks for IDs, systemBlocks for actual content
	var sysBlocks []BlueprintBlock
	sysIdx := 0
	for _, nb := range namedBlocks {
		if nb.ID == cacheBoundaryID {
			continue
		}
		if nb.Text == "" {
			continue
		}
		h := shortHash(nb.Text)
		tokens := estimateTokensStr(nb.Text)
		cache := ""
		// Check if this system block has cache_control set
		if sysIdx < len(systemBlocks) && systemBlocks[sysIdx].CacheControl.Type != "" {
			cache = "BP2"
		}
		id := nb.ID
		if len(id) > 40 {
			id = id[:40]
		}
		sysBlocks = append(sysBlocks, BlueprintBlock{
			ID: fmt.Sprintf("sys:%s", id), Hash: h, Tokens: tokens, Cache: cache,
		})
		sysIdx++
	}
	bp.Sections = append(bp.Sections, BlueprintSection{Name: "system", Blocks: sysBlocks})

	// Messages section. Ask the placement rule where the breakpoints go rather
	// than restating it: this used to assume the second-to-last message and so
	// reported a position the request had not used since the frozen zone landed.
	historyBreakpoints := map[int]bool{}
	for _, idx := range agent.HistoryCacheBreakpointIndices(len(messages), frozenIdx, len(messages)-1) {
		historyBreakpoints[idx] = true
	}

	var msgBlocks []BlueprintBlock
	for i, m := range messages {
		content := messageContent(m)
		h := shortHash(content)
		tokens := estimateTokensStr(content)
		cache := ""
		if historyBreakpoints[i] {
			cache = "BP3"
		}
		role := string(m.Role)
		// Identify message type more specifically
		msgType := role
		if m.Role == anthropic.MessageParamRoleUser && hasToolResultContent(m) {
			msgType = "tool_result"
		} else if m.Role == anthropic.MessageParamRoleAssistant && hasToolUseContent(m) {
			msgType = "tool_use"
		}
		msgBlocks = append(msgBlocks, BlueprintBlock{
			ID: fmt.Sprintf("msg[%d]:%s", i, msgType), Hash: h, Tokens: tokens, Cache: cache,
		})
	}
	bp.Sections = append(bp.Sections, BlueprintSection{Name: "messages", Blocks: msgBlocks})

	return bp
}

// DiffBlueprints compares two blueprints and predicts cache behavior.
// Cache rules:
// - Prefix match: everything up to a breakpoint must be byte-identical
// - If any block before a BP changes, that BP and all downstream BPs miss
// - Blocks after the last BP are always uncached
//
// Blocks are matched BY POSITION within their section, never by ID. A prefix
// cache is a comparison of ordered bytes, so the only question this can ask is
// "does the same content still sit at the same offset". Matching on ID answered
// a different question and got both directions wrong: two memories swapping
// places, or one dropping out, kept every ID matched and reported a hit the
// request never got, while a memory whose importance drifted into a new label
// reported its unchanged text as a brand-new block.
func DiffBlueprints(prev, curr *PromptBlueprint) *BlueprintDiff {
	diff := &BlueprintDiff{
		Turn:     curr.Turn,
		PrevTurn: prev.Turn,
	}

	prevSections := make(map[string][]BlueprintBlock, len(prev.Sections))
	for _, s := range prev.Sections {
		prevSections[s.Name] = s.Blocks
	}

	// Offset, in the flattened request, of the first block whose bytes differ
	// from last turn's. Everything from there on is outside the cached prefix,
	// whatever its own content says. Negative until something diverges.
	divergence := -1
	noteDivergence := func(at int) {
		if divergence < 0 {
			divergence = at
		}
	}

	flat := 0
	seen := make(map[string]bool, len(curr.Sections))

	for _, cs := range curr.Sections {
		seen[cs.Name] = true
		ds := DiffSection{Name: cs.Name}
		prevBlocks := prevSections[cs.Name]

		for i, cb := range cs.Blocks {
			db := DiffBlock{
				ID:     cb.ID,
				Hash:   cb.Hash,
				Tokens: cb.Tokens,
				Cache:  cb.Cache,
			}

			switch {
			case i >= len(prevBlocks):
				db.Status = "NEW"
				noteDivergence(flat)
			case prevBlocks[i].Hash != cb.Hash:
				db.Status = "CHANGED"
				db.PrevHash = prevBlocks[i].Hash
				noteDivergence(flat)
			case divergence >= 0:
				db.Status = "CASCADE" // content same but prefix changed upstream
			default:
				db.Status = "STABLE"
			}

			ds.Blocks = append(ds.Blocks, db)
			flat++
		}

		// Blocks last turn sent that this one does not. They carry no tokens in
		// this request, but the prefix broke where the first of them used to
		// sit — which is the offset the next section now starts at, so nothing
		// downstream would notice on its own.
		if len(prevBlocks) > len(cs.Blocks) {
			noteDivergence(flat)
			for _, pb := range prevBlocks[len(cs.Blocks):] {
				// No Cache: whatever breakpoint this block carried belongs to
				// last turn's request, and printing it here would advertise a
				// breakpoint this one does not have.
				ds.Blocks = append(ds.Blocks, DiffBlock{
					ID:       pb.ID,
					PrevHash: pb.Hash,
					Status:   "REMOVED",
				})
			}
		}

		diff.Sections = append(diff.Sections, ds)
	}

	// A whole section disappearing is not a shape BuildBlueprint produces — it
	// always emits tools, system and messages. If one ever does, say the prefix
	// is gone rather than quietly reporting a hit on the sections that remain.
	for _, s := range prev.Sections {
		if seen[s.Name] {
			continue
		}
		noteDivergence(0)
		ds := DiffSection{Name: s.Name}
		for _, pb := range s.Blocks {
			ds.Blocks = append(ds.Blocks, DiffBlock{
				ID:       pb.ID,
				PrevHash: pb.Hash,
				Status:   "REMOVED",
			})
		}
		diff.Sections = append(diff.Sections, ds)
	}

	computeDiffSummary(diff, divergence)

	return diff
}

// computeDiffSummary settles each block's cache prediction and totals the
// tokens. divergence is the flattened offset of the first block that broke the
// prefix, or negative if nothing did.
//
// Every token in the request is read from cache, written to cache, or uncached
// — exactly one of the three — so the three add up to the total. They did not
// before: DiffBlueprints had already banked a breakpoint's tokens before this
// ran, and this counted them a second time.
func computeDiffSummary(diff *BlueprintDiff, divergence int) {
	// The blocks this request actually carries, in request order. A REMOVED
	// block is last turn's and is not sent, so it owes no tokens.
	var blocks []*DiffBlock
	for si := range diff.Sections {
		for bi := range diff.Sections[si].Blocks {
			b := &diff.Sections[si].Blocks[bi]
			if b.Status == "REMOVED" {
				continue
			}
			blocks = append(blocks, b)
		}
	}

	if divergence < 0 {
		divergence = len(blocks)
	}

	// The read comes back from the last breakpoint whose whole prefix survived;
	// the write runs from there to the last breakpoint in the request; anything
	// past that last breakpoint is never cached at all.
	lastBreakpoint, lastSurvivingBreakpoint := -1, -1
	for i, b := range blocks {
		if b.Cache == "" {
			continue
		}
		lastBreakpoint = i
		if i < divergence {
			lastSurvivingBreakpoint = i
		}
	}

	for i, b := range blocks {
		diff.Summary.TotalTokens += b.Tokens

		switch {
		case i > lastBreakpoint:
			diff.Summary.Uncached += b.Tokens
		case i <= lastSurvivingBreakpoint:
			diff.Summary.CachedRead += b.Tokens
		default:
			diff.Summary.CachedWrite += b.Tokens
		}

		if b.Cache != "" {
			if i <= lastSurvivingBreakpoint {
				b.Status = "HIT"
			} else {
				b.Status = "WRITE" // breakpoint will be rewritten
			}
		}

		// Stable means the bytes are unchanged AND still inside the surviving
		// prefix. A CASCADE block is byte-identical and still has to be re-sent,
		// which is the thing worth counting.
		switch b.Status {
		case "STABLE", "HIT":
			diff.Summary.BlocksStable++
		default:
			diff.Summary.BlocksChanged++
		}
	}
}

// FormatBlueprint returns a compact human-readable representation.
func FormatBlueprint(bp *PromptBlueprint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "── Blueprint (turn %d) ──\n", bp.Turn)
	for _, s := range bp.Sections {
		totalTok := 0
		for _, bl := range s.Blocks {
			totalTok += bl.Tokens
		}
		fmt.Fprintf(&b, "┌ %s (%d blocks, ~%d tok)\n", s.Name, len(s.Blocks), totalTok)
		for _, bl := range s.Blocks {
			cache := ""
			if bl.Cache != "" {
				cache = " ●" + bl.Cache
			}
			fmt.Fprintf(&b, "│  %s  %s  ~%d tok%s\n", bl.Hash, bl.ID, bl.Tokens, cache)
		}
		b.WriteString("└\n")
	}
	return b.String()
}

// FormatDiff returns a compact human-readable diff with cache predictions.
func FormatDiff(d *BlueprintDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "── Diff (turn %d vs %d) ──\n", d.PrevTurn, d.Turn)

	for _, s := range d.Sections {
		fmt.Fprintf(&b, "┌ %s\n", s.Name)
		for _, bl := range s.Blocks {
			icon := statusIcon(bl.Status)
			cache := ""
			if bl.Cache != "" {
				cache = " ●" + bl.Cache
			}
			prev := ""
			if bl.PrevHash != "" {
				prev = fmt.Sprintf(" (was %s)", bl.PrevHash)
			}
			// A removed block is not in this request, so it has no size here.
			// "~0 tok" would read as an empty block rather than an absent one.
			size := fmt.Sprintf("~%d tok", bl.Tokens)
			if bl.Status == "REMOVED" {
				size = "gone"
			}
			fmt.Fprintf(&b, "│ %s %s  %s  %s%s%s\n",
				icon, bl.Hash, bl.ID, size, cache, prev)
		}
		b.WriteString("└\n")
	}

	fmt.Fprintf(&b, "── Summary: %d tok total | read:%d write:%d uncached:%d | %d stable, %d changed ──\n",
		d.Summary.TotalTokens, d.Summary.CachedRead, d.Summary.CachedWrite,
		d.Summary.Uncached, d.Summary.BlocksStable, d.Summary.BlocksChanged)

	return b.String()
}

func statusIcon(status string) string {
	switch status {
	case "STABLE", "HIT":
		return "✅"
	case "WRITE":
		return "📝"
	case "CHANGED":
		return "🔄"
	case "CASCADE":
		return "💥"
	case "NEW":
		return "🆕"
	case "REMOVED":
		return "❌"
	default:
		return "  "
	}
}

// shortHash returns first 8 hex chars of SHA-256.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

// estimateTokensStr estimates tokens from string length (~4 chars/token).
func estimateTokensStr(s string) int {
	return (len(s) + 3) / 4
}

// messageContent extracts a string representation of message content for hashing.
func messageContent(m anthropic.MessageParam) string {
	var parts []string
	for _, block := range m.Content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		} else if block.OfToolUse != nil {
			// Render as JSON text, not %v: the input is a json.RawMessage, and
			// %v prints its decimal byte codes — roughly 3.7 characters per
			// byte, which inflates this message's token estimate by the same
			// factor.
			parts = append(parts, block.OfToolUse.Name+":"+conversation.ToolInputText(block.OfToolUse.Input))
		} else if block.OfToolResult != nil {
			parts = append(parts, "result:"+block.OfToolResult.ToolUseID)
			for _, c := range block.OfToolResult.Content {
				if c.OfText != nil {
					parts = append(parts, c.OfText.Text)
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

// hasToolResultContent checks if a message contains tool_result blocks.
func hasToolResultContent(m anthropic.MessageParam) bool {
	for _, block := range m.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
}

// hasToolUseContent checks if a message contains tool_use blocks.
func hasToolUseContent(m anthropic.MessageParam) bool {
	for _, block := range m.Content {
		if block.OfToolUse != nil {
			return true
		}
	}
	return false
}
