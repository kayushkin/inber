package main

import (
	"fmt"
	"strings"

	"github.com/kayushkin/inber/agent"
)

// ANSI color helpers
const (
	reset   = "\033[0m"
	dim     = "\033[2m"
	bold    = "\033[1m"
	italic  = "\033[3m"
	cyan    = "\033[36m"
	magenta = "\033[35m"
	yellow  = "\033[33m"
	green   = "\033[32m"
	red     = "\033[31m"
	blue    = "\033[34m"
)

// DisplayToolCall prints a tool call to the terminal.
func DisplayToolCall(name string, input string) {
	fmt.Printf("\n%s⚡ %s%s", magenta+bold, name, reset)
	// Show a compact version of the input
	summary := summarizeInput(input)
	if summary != "" {
		fmt.Printf(" %s%s%s", dim, summary, reset)
	}
	fmt.Println()
}

// DisplayToolResult prints a tool result to the terminal.
func DisplayToolResult(name string, output string, isError bool) {
	if isError {
		fmt.Printf("%s  ✗ %s%s\n", red, truncate(output, 200), reset)
		return
	}
	lines := strings.Count(output, "\n") + 1
	bytes := len(output)
	fmt.Printf("%s  → %d lines, %d bytes%s\n", dim, lines, bytes, reset)
}

// DisplayThinking prints thinking/reasoning text.
func DisplayThinking(text string) {
	if text == "" {
		return
	}
	fmt.Printf("\n%s%s💭 thinking...%s\n", dim, italic, reset)
	// Show truncated thinking — full text goes to logs
	lines := strings.Split(text, "\n")
	max := 8
	if len(lines) <= max {
		for _, l := range lines {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
	} else {
		for _, l := range lines[:3] {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
		fmt.Printf("%s%s  ... (%d more lines)%s\n", dim, italic, len(lines)-6, reset)
		for _, l := range lines[len(lines)-3:] {
			fmt.Printf("%s%s  %s%s\n", dim, italic, l, reset)
		}
	}
}

// DisplayResponse prints the final assistant response.
func DisplayResponse(text string) {
	fmt.Printf("\n%s\n", text)
}

// DisplayStats prints token usage and cost.
func DisplayStats(result *agent.TurnResult, model string) {
	cost := calcCost(model, result.InputTokens, result.OutputTokens)
	parts := []string{
		fmt.Sprintf("in=%d", result.InputTokens),
		fmt.Sprintf("out=%d", result.OutputTokens),
	}
	if result.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("tools=%d", result.ToolCalls))
	}
	if cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", cost))
	}
	fmt.Printf("%s[%s]%s\n", dim, strings.Join(parts, " | "), reset)
}

func calcCost(model string, inTok, outTok int) float64 {
	info, ok := agent.Models[model]
	if !ok {
		return 0
	}
	return (float64(inTok)*info.InputCostPer1M + float64(outTok)*info.OutputCostPer1M) / 1_000_000
}

func summarizeInput(raw string) string {
	// Quick extraction of key fields for display
	s := strings.TrimSpace(raw)
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	// Clean up JSON for display
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
