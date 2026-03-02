package session

import (
	"fmt"
	"strings"
)

// TruncateStrategy defines how content should be truncated.
type TruncateStrategy string

const (
	StrategyNone     TruncateStrategy = "none"      // no truncation
	StrategyHeadTail TruncateStrategy = "head-tail" // first N + last M tokens
	StrategyAuto     TruncateStrategy = "auto"      // detect best strategy
)

// TruncateConfig controls truncation behavior.
type TruncateConfig struct {
	Threshold  int              // truncate if > N tokens
	HeadTokens int              // show first N tokens
	TailTokens int              // show last N tokens
	Strategy   TruncateStrategy // which strategy to use
	CreateRef  bool             // create memory reference for full content
}

// TruncateResult contains the result of truncation.
type TruncateResult struct {
	Truncated   bool   // was truncation applied?
	Original    string // original content
	Displayed   string // what goes in context
	RefID       string // memory reference ID (if created)
	SavedTokens int    // tokens saved by truncation
}

// DefaultTruncateConfig returns sensible defaults for truncation.
func DefaultTruncateConfig() TruncateConfig {
	return TruncateConfig{
		Threshold:  1000, // truncate if > 1K tokens
		HeadTokens: 500,  // first 500 tokens
		TailTokens: 200,  // last 200 tokens
		Strategy:   StrategyAuto,
		CreateRef:  true,
	}
}

// EstimateTokens roughly estimates token count (4 chars ≈ 1 token).
func EstimateTokens(text string) int {
	return len(text) / 4
}

// TruncateToolResult truncates large tool output according to config.
func TruncateToolResult(toolName, output string, cfg TruncateConfig) TruncateResult {
	tokens := EstimateTokens(output)

	// No truncation needed
	if tokens <= cfg.Threshold {
		return TruncateResult{
			Truncated: false,
			Original:  output,
			Displayed: output,
		}
	}

	// Apply truncation strategy
	var displayed string
	switch cfg.Strategy {
	case StrategyNone:
		displayed = output
	case StrategyAuto:
		// Auto-detect best strategy based on content
		strategy := detectStrategy(toolName, output)
		if strategy == StrategyHeadTail {
			displayed = truncateHeadTail(output, cfg.HeadTokens, cfg.TailTokens)
		} else {
			displayed = truncateHeadTail(output, cfg.HeadTokens, cfg.TailTokens)
		}
	case StrategyHeadTail:
		displayed = truncateHeadTail(output, cfg.HeadTokens, cfg.TailTokens)
	default:
		displayed = truncateHeadTail(output, cfg.HeadTokens, cfg.TailTokens)
	}

	savedTokens := tokens - EstimateTokens(displayed)

	return TruncateResult{
		Truncated:   true,
		Original:    output,
		Displayed:   displayed,
		SavedTokens: savedTokens,
	}
}

// truncateHeadTail shows first N + last M tokens worth of content.
func truncateHeadTail(content string, headTokens, tailTokens int) string {
	// Convert tokens to approximate character counts
	headChars := headTokens * 4
	tailChars := tailTokens * 4

	if len(content) <= headChars+tailChars {
		return content
	}

	// Find good break points (try to break on newlines)
	head := content[:headChars]
	tail := content[len(content)-tailChars:]

	// Try to break on newline for cleaner display
	if idx := strings.LastIndex(head, "\n"); idx > headChars/2 {
		head = head[:idx]
	}
	if idx := strings.Index(tail, "\n"); idx > 0 && idx < tailChars/2 {
		tail = tail[idx+1:]
	}

	totalTokens := EstimateTokens(content)
	savedTokens := totalTokens - headTokens - tailTokens
	omittedLines := strings.Count(content[len(head):len(content)-len(tail)], "\n")

	var truncationMsg string
	if omittedLines > 0 {
		truncationMsg = fmt.Sprintf("\n\n[... truncated %d tokens (%d lines) ...]\n\n", savedTokens, omittedLines)
	} else {
		truncationMsg = fmt.Sprintf("\n\n[... truncated %d tokens ...]\n\n", savedTokens)
	}

	return head + truncationMsg + tail
}

// detectStrategy analyzes content to pick the best truncation strategy.
func detectStrategy(toolName, output string) TruncateStrategy {
	// For now, always use head-tail
	// Phase 2 will add smart strategies for errors, files, etc.
	return StrategyHeadTail
}

// TruncateConfigForRole returns truncation config based on agent role.
func TruncateConfigForRole(role string) TruncateConfig {
	switch strings.ToLower(role) {
	case "main", "agent":
		// Main agent: aggressive truncation (low token budget)
		return TruncateConfig{
			Threshold:  1000,
			HeadTokens: 500,
			TailTokens: 200,
			Strategy:   StrategyAuto,
			CreateRef:  true,
		}
	case "project":
		// Project agent: moderate (needs more context)
		return TruncateConfig{
			Threshold:  3000,
			HeadTokens: 1500,
			TailTokens: 500,
			Strategy:   StrategyAuto,
			CreateRef:  true,
		}
	case "run":
		// Run agent: minimal truncation (expects large output)
		return TruncateConfig{
			Threshold:  5000,
			HeadTokens: 2000,
			TailTokens: 1000,
			Strategy:   StrategyHeadTail,
			CreateRef:  false,
		}
	default:
		return DefaultTruncateConfig()
	}
}
