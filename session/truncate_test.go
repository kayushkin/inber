package session

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateToolResult_NoTruncation(t *testing.T) {
	cfg := TruncateConfig{
		Threshold:  1000,
		HeadTokens: 500,
		TailTokens: 200,
		Strategy:   StrategyHeadTail,
	}

	output := "small output"
	result := TruncateToolResult("shell", output, cfg)

	if result.Truncated {
		t.Error("expected no truncation for small output")
	}
	if result.Displayed != output {
		t.Errorf("displayed should equal original, got: %s", result.Displayed)
	}
	if result.SavedTokens != 0 {
		t.Errorf("expected 0 saved tokens, got: %d", result.SavedTokens)
	}
}

func TestTruncateToolResult_HeadTail(t *testing.T) {
	cfg := TruncateConfig{
		Threshold:  100,  // 100 tokens = ~400 chars
		HeadTokens: 50,   // 50 tokens = ~200 chars
		TailTokens: 20,   // 20 tokens = ~80 chars
		Strategy:   StrategyHeadTail,
	}

	// Create a large output (~1000 tokens = ~4000 chars)
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, strings.Repeat("error line ", 4)) // ~40 chars per line
	}
	output := strings.Join(lines, "\n")

	result := TruncateToolResult("shell", output, cfg)

	if !result.Truncated {
		t.Error("expected truncation for large output")
	}

	// Should have head + truncation message + tail
	if !strings.Contains(result.Displayed, "[... truncated") {
		t.Error("expected truncation message in output")
	}

	// Should be significantly smaller
	if EstimateTokens(result.Displayed) > EstimateTokens(output)/2 {
		t.Errorf("truncated output should be much smaller, got %d tokens vs original %d",
			EstimateTokens(result.Displayed), EstimateTokens(output))
	}

	// Should report saved tokens
	if result.SavedTokens <= 0 {
		t.Errorf("expected positive saved tokens, got: %d", result.SavedTokens)
	}
}

func TestTruncateToolResult_BreaksOnNewlines(t *testing.T) {
	cfg := TruncateConfig{
		Threshold:  100,
		HeadTokens: 50,
		TailTokens: 20,
		Strategy:   StrategyHeadTail,
	}

	// Create output with clear line structure
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line "+strings.Repeat("x", 40))
	}
	output := strings.Join(lines, "\n")

	result := TruncateToolResult("shell", output, cfg)

	// Head should end at a newline (not mid-word)
	headPart := strings.Split(result.Displayed, "[... truncated")[0]
	if !strings.HasSuffix(headPart, "\n") && !strings.HasSuffix(headPart, "x") {
		t.Error("head should break on newline boundary")
	}
}

func TestTruncateConfigForRole(t *testing.T) {
	tests := []struct {
		role      string
		threshold int
	}{
		{"main", 1000},
		{"agent", 1000},
		{"project", 3000},
		{"run", 5000},
		{"unknown", 1000}, // defaults to standard config
	}

	for _, tt := range tests {
		cfg := TruncateConfigForRole(tt.role)
		if cfg.Threshold != tt.threshold {
			t.Errorf("role %s: expected threshold %d, got %d",
				tt.role, tt.threshold, cfg.Threshold)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text   string
		tokens int
	}{
		{"", 0},
		{"test", 1},
		{"hello world", 2},
		{strings.Repeat("x", 400), 100},
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.text)
		if got != tt.tokens {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.text, got, tt.tokens)
		}
	}
}

func TestTruncateToolResult_PreservesContent(t *testing.T) {
	cfg := TruncateConfig{
		Threshold:  100,
		HeadTokens: 50,
		TailTokens: 20,
		Strategy:   StrategyHeadTail,
	}

	// Create output with distinctive head and tail
	var lines []string
	lines = append(lines, "HEADER LINE 1", "HEADER LINE 2", "HEADER LINE 3")
	for i := 0; i < 50; i++ {
		lines = append(lines, "middle content line "+strings.Repeat("x", 30))
	}
	lines = append(lines, "FOOTER LINE 1", "FOOTER LINE 2", "FOOTER LINE 3")
	output := strings.Join(lines, "\n")

	result := TruncateToolResult("shell", output, cfg)

	// Should preserve header
	if !strings.Contains(result.Displayed, "HEADER LINE 1") {
		t.Error("should preserve header content")
	}

	// Should preserve footer
	if !strings.Contains(result.Displayed, "FOOTER LINE 3") {
		t.Error("should preserve footer content")
	}

	// Should show truncation happened
	if !strings.Contains(result.Displayed, "truncated") {
		t.Error("should indicate truncation")
	}
}

// Head and tail are cut at byte offsets derived from a token budget, so a
// multibyte rune can straddle either cut. What comes back is tool output the
// model reads.
//
// Two properties of the fixture are load-bearing, and the version this replaced
// had neither, so it passed against a raw s[:n] and pinned nothing:
//
//   - The runes are three bytes wide. The budget is always headTokens*4, so a
//     two-byte rune can never straddle it — every cut lands between runes. The
//     old fixture was "héllo wörld\n" repeated, whose only wide runes are
//     two-byte, and not one of its five budgets cut inside a rune at the head.
//   - The fixture carries no newline. truncateHeadTail follows each cut with a
//     newline-break step — head back to its last newline, tail forward past its
//     first — and either step discards the partial rune a byte cut leaves
//     behind. The old fixture's one straddling budget was laundered that way, so
//     even the cut that did land mid-rune came back valid.
//
// len(content) is a multiple of three, which makes a budget divisible by three
// land on a rune boundary at both ends — that is the known-negative control, and
// the counters below fail the test rather than let either case go unexercised.
func TestTruncateHeadTailCutsOnRuneBoundaries(t *testing.T) {
	const maxTokens = 60
	content := strings.Repeat("日本語", 200) + "🦀ab"

	straddled, aligned := 0, 0
	for headTokens := 1; headTokens <= maxTokens; headTokens++ {
		budget := headTokens * 4
		if utf8.RuneStart(content[budget]) && utf8.RuneStart(content[len(content)-budget]) {
			aligned++
		} else {
			straddled++
		}
		got := truncateHeadTail(content, headTokens, headTokens)
		if !utf8.ValidString(got) {
			t.Fatalf("truncateHeadTail(head=tail=%d) is not valid UTF-8", headTokens)
		}
	}

	if straddled == 0 {
		t.Fatalf("no budget in 1..%d cut inside a rune — the fixture would pass against a raw byte cut", maxTokens)
	}
	if aligned == 0 {
		t.Fatalf("no budget in 1..%d landed on a rune boundary — the control proved nothing", maxTokens)
	}
}
