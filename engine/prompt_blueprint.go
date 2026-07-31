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
	Turn     int              `json:"turn"`
	Sections []BlueprintSection `json:"sections"`
}

// BlueprintSection groups blocks by their role in the request.
type BlueprintSection struct {
	Name   string           `json:"name"`
	Blocks []BlueprintBlock `json:"blocks"`
}

// BlueprintBlock represents one content block in the prompt with its hash
// and cache metadata.
type BlueprintBlock struct {
	ID       string `json:"id"`                  // block identifier (tool name, memory ID, msg role+index)
	Hash     string `json:"hash"`                // first 8 chars of SHA-256
	Tokens   int    `json:"tokens"`              // estimated token count
	Cache    string `json:"cache,omitempty"`      // "BP1"/"BP2"/"BP3" if breakpoint, empty otherwise
	Status   string `json:"status,omitempty"`     // "HIT"/"MISS"/"WRITE"/"UNCACHED" (set by diff)
}

// BlueprintDiff compares two blueprints and annotates cache behavior.
type BlueprintDiff struct {
	Turn         int    `json:"turn"`
	PrevTurn     int    `json:"prev_turn"`
	Sections     []DiffSection `json:"sections"`
	Summary      DiffSummary   `json:"summary"`
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
	Status   string `json:"status"`  // HIT, MISS, WRITE, NEW, REMOVED, UNCACHED
	PrevHash string `json:"prev_hash,omitempty"` // if changed
}

type DiffSummary struct {
	TotalTokens    int `json:"total_tokens"`
	CachedRead     int `json:"cached_read"`     // tokens predicted to hit cache
	CachedWrite    int `json:"cached_write"`    // tokens predicted to be written to cache
	Uncached       int `json:"uncached"`        // tokens with no cache control
	BlocksChanged  int `json:"blocks_changed"`
	BlocksStable   int `json:"blocks_stable"`
}

// BuildBlueprint creates a structural manifest of the current prompt.
func BuildBlueprint(
	turn int,
	tools []agent.Tool,
	systemBlocks []anthropic.TextBlockParam,
	namedBlocks []sessionMod.NamedBlock,
	messages []anthropic.MessageParam,
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

	// Messages section
	var msgBlocks []BlueprintBlock
	for i, m := range messages {
		content := messageContent(m)
		h := shortHash(content)
		tokens := estimateTokensStr(content)
		cache := ""
		// BP3 is on second-to-last message
		if i == len(messages)-2 {
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
func DiffBlueprints(prev, curr *PromptBlueprint) *BlueprintDiff {
	diff := &BlueprintDiff{
		Turn:     curr.Turn,
		PrevTurn: prev.Turn,
	}

	// Build lookup of previous blocks by section+id
	prevMap := make(map[string]string) // section:id -> hash
	for _, s := range prev.Sections {
		for _, b := range s.Blocks {
			key := s.Name + ":" + b.ID
			prevMap[key] = b.Hash
		}
	}

	// Track if prefix has been invalidated (cascade)
	prefixInvalidated := false

	for _, cs := range curr.Sections {
		ds := DiffSection{Name: cs.Name}
		for _, cb := range cs.Blocks {
			key := cs.Name + ":" + cb.ID
			prevHash, existed := prevMap[key]

			db := DiffBlock{
				ID:     cb.ID,
				Hash:   cb.Hash,
				Tokens: cb.Tokens,
				Cache:  cb.Cache,
			}

			if !existed {
				db.Status = "NEW"
				prefixInvalidated = true
			} else if prevHash != cb.Hash {
				db.Status = "CHANGED"
				db.PrevHash = prevHash
				prefixInvalidated = true
			} else if prefixInvalidated {
				db.Status = "CASCADE" // content same but prefix changed upstream
			} else {
				db.Status = "STABLE"
			}

			// Determine cache prediction
			if cb.Cache != "" {
				if prefixInvalidated {
					db.Status = "WRITE" // breakpoint will be rewritten
					diff.Summary.CachedWrite += cb.Tokens
				} else {
					db.Status = "HIT"
					diff.Summary.CachedRead += cb.Tokens
				}
			} else if cb.Cache == "" && !prefixInvalidated {
				// Between a hit BP and the next section — still in cached prefix
				// (handled by cumulative prefix logic below)
			}

			ds.Blocks = append(ds.Blocks, db)
		}
		diff.Sections = append(diff.Sections, ds)
	}

	// Second pass: compute summary with proper prefix accounting
	computeDiffSummary(diff)

	return diff
}

// computeDiffSummary fills in the summary with token counts.
func computeDiffSummary(diff *BlueprintDiff) {
	for _, s := range diff.Sections {
		for _, b := range s.Blocks {
			diff.Summary.TotalTokens += b.Tokens
			switch b.Status {
			case "STABLE", "HIT":
				diff.Summary.BlocksStable++
				diff.Summary.CachedRead += b.Tokens
			case "CHANGED", "NEW", "CASCADE", "WRITE":
				diff.Summary.BlocksChanged++
				if b.Cache != "" {
					diff.Summary.CachedWrite += b.Tokens
				} else {
					diff.Summary.Uncached += b.Tokens
				}
			}
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
			fmt.Fprintf(&b, "│ %s %s  %s  ~%d tok%s%s\n",
				icon, bl.Hash, bl.ID, bl.Tokens, cache, prev)
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
