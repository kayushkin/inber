package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// NamedBlock is a system prompt chunk with an ID for labeling in breakdowns.
type NamedBlock struct {
	ID   string
	Text string
}

// promptState tracks what was in the previous turn so we can diff.
var prevMessageCount int

// WritePromptBreakdown writes prompt files for a given turn.
//
// Turn 1 writes:
//   - prompts/system.md   — full system prompt (shared, written once)
//   - prompts/tools.md    — tool definitions (shared, written once)
//   - prompts/turn-1.md   — messages + token summary
//
// Turn 2+ writes:
//   - prompts/turn-N.md   — new messages only (diff) + token summary
func WritePromptBreakdown(logFilePath string, sessionID string, turn int, params *anthropic.MessageNewParams, blockNames []NamedBlock) error {
	sessionDir := filepath.Dir(logFilePath)
	promptsDir := filepath.Join(sessionDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}

	// Write shared files on turn 1
	if turn == 1 {
		writeSystemFiles(promptsDir, params.System, blockNames)
		writeToolsFile(promptsDir, params.Tools)
	}

	// Write per-turn file
	filename := fmt.Sprintf("turn-%d.md", turn)
	path := filepath.Join(promptsDir, filename)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Turn %d\n\n", turn))
	sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Model:** %s\n\n", params.Model))

	systemTokens := estimateSystemTokens(params.System)
	toolTokens := estimateToolTokens(params.Tools)

	if turn == 1 {
		writeAllMessages(&sb, params.Messages, 0)
	} else {
		newStart := prevMessageCount
		if newStart > len(params.Messages) {
			newStart = 0
		}
		newCount := len(params.Messages) - newStart
		sb.WriteString(fmt.Sprintf("## New Messages (+%d, %d total)\n\n", newCount, len(params.Messages)))
		writeAllMessages(&sb, params.Messages, newStart)
	}

	// Token summary
	messageTokens := estimateMessageTokens(params.Messages)
	sb.WriteString(fmt.Sprintf("\n## Tokens\n\n"))
	sb.WriteString(fmt.Sprintf("| Section | Tokens (est) |\n"))
	sb.WriteString(fmt.Sprintf("|---------|-------------|\n"))
	sb.WriteString(fmt.Sprintf("| [System prompt](system.md) | ~%d (%d blocks) |\n", systemTokens, len(params.System)))
	sb.WriteString(fmt.Sprintf("| [Tools](tools.md) | ~%d (%d tools) |\n", toolTokens, len(params.Tools)))
	sb.WriteString(fmt.Sprintf("| Messages | ~%d (%d messages) |\n", messageTokens, len(params.Messages)))
	sb.WriteString(fmt.Sprintf("| **Total** | **~%d** |\n", systemTokens+messageTokens+toolTokens))

	prevMessageCount = len(params.Messages)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// writeSystemFiles writes each system prompt block as a separate file + an index.
//
//	prompts/system.md          — index with links and token counts
//	prompts/system-01-identity.md  — individual block files
func writeSystemFiles(promptsDir string, blocks []anthropic.TextBlockParam, blockNames []NamedBlock) {
	if len(blocks) == 0 {
		return
	}

	var index strings.Builder
	index.WriteString("# System Prompt\n\n")
	index.WriteString(fmt.Sprintf("%d blocks\n\n", len(blocks)))
	index.WriteString("| # | Block | Tokens (est) |\n")
	index.WriteString("|---|-------|-------------|\n")

	totalTokens := 0
	for i, block := range blocks {
		tokens := len(block.Text) / 4
		totalTokens += tokens

		// Determine block name/slug
		name := fmt.Sprintf("block-%d", i+1)
		if i < len(blockNames) && blockNames[i].ID != "" {
			name = blockNames[i].ID
		}

		// Slugify for filename
		slug := slugify(name)
		filename := fmt.Sprintf("system-%02d-%s.md", i+1, slug)

		// Write individual block file
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", name))
		sb.WriteString(fmt.Sprintf("*~%d tokens*\n\n", tokens))
		sb.WriteString(block.Text)
		os.WriteFile(filepath.Join(promptsDir, filename), []byte(sb.String()), 0644)

		// Add to index
		index.WriteString(fmt.Sprintf("| %d | [%s](%s) | ~%d |\n", i+1, name, filename, tokens))
	}

	index.WriteString(fmt.Sprintf("\n**Total:** ~%d tokens\n", totalTokens))
	os.WriteFile(filepath.Join(promptsDir, "system.md"), []byte(index.String()), 0644)
}

// slugify converts a name to a filename-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' || r == '/' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// writeToolsFile writes prompts/tools.md with tool definitions.
func writeToolsFile(promptsDir string, tools []anthropic.ToolUnionParam) {
	var sb strings.Builder
	sb.WriteString("# Tool Definitions\n\n")
	if len(tools) == 0 {
		sb.WriteString("*No tools registered.*\n")
		os.WriteFile(filepath.Join(promptsDir, "tools.md"), []byte(sb.String()), 0644)
		return
	}

	sb.WriteString(fmt.Sprintf("%d tools registered\n\n", len(tools)))
	for i, tool := range tools {
		if tool.OfTool != nil {
			t := tool.OfTool
			sb.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, t.Name))
			desc := t.Description.Or("")
			if desc != "" {
				sb.WriteString(desc + "\n\n")
			}
			// Schema as JSON
			schemaJSON, err := json.MarshalIndent(t.InputSchema, "", "  ")
			if err == nil && len(schemaJSON) > 2 { // skip empty "{}"
				sb.WriteString("**Schema:**\n\n```json\n")
				sb.WriteString(string(schemaJSON))
				sb.WriteString("\n```\n\n")
			}
		}
	}

	totalTokens := estimateToolTokens(tools)
	sb.WriteString(fmt.Sprintf("---\n**Total:** %d tools, ~%d tokens\n", len(tools), totalTokens))
	os.WriteFile(filepath.Join(promptsDir, "tools.md"), []byte(sb.String()), 0644)
}

func estimateToolTokens(tools []anthropic.ToolUnionParam) int {
	total := 0
	for _, tool := range tools {
		total += 50
		if tool.OfTool != nil {
			desc := tool.OfTool.Description.Or("")
			total += len(desc) / 4
			// Rough schema estimate from JSON size
			if data, err := json.Marshal(tool.OfTool.InputSchema); err == nil {
				total += len(data) / 4
			}
		}
	}
	return total
}

func writeSystemPrompt(sb *strings.Builder, blocks []anthropic.TextBlockParam, blockNames []NamedBlock) {
	if len(blocks) == 0 {
		return
	}
	sb.WriteString("## System Prompt\n\n")
	for i, block := range blocks {
		tokens := len(block.Text) / 4
		label := ""
		if i < len(blockNames) && blockNames[i].ID != "" {
			label = " — " + blockNames[i].ID
		}
		sb.WriteString(fmt.Sprintf("### Block %d%s (~%d tokens)\n\n", i+1, label, tokens))
		sb.WriteString(block.Text + "\n\n")
	}
}

func writeAllMessages(sb *strings.Builder, messages []anthropic.MessageParam, startFrom int) {
	if startFrom >= len(messages) {
		sb.WriteString("*(no new messages)*\n")
		return
	}
	sb.WriteString("| # | Role | Tokens (est) | Content |\n")
	sb.WriteString("|---|------|-------------|--------|\n")
	for i := startFrom; i < len(messages); i++ {
		msg := messages[i]
		role := string(msg.Role)
		msgTokens := 4
		var contentPreview string

		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				msgTokens += len(text) / 4
				if len(text) > 80 {
					contentPreview = strings.ReplaceAll(text[:80], "|", "\\|") + "..."
				} else {
					contentPreview = strings.ReplaceAll(text, "|", "\\|")
				}
			} else if block.OfToolUse != nil {
				msgTokens += 50
				contentPreview = fmt.Sprintf("[tool_use: %s]", block.OfToolUse.Name)
			} else if block.OfToolResult != nil {
				msgTokens += 50
				contentPreview = "[tool_result]"
			}
		}
		contentPreview = strings.ReplaceAll(contentPreview, "\n", " ")
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %s |\n", i+1, role, msgTokens, contentPreview))
	}
}

func estimateSystemTokens(blocks []anthropic.TextBlockParam) int {
	total := 0
	for _, b := range blocks {
		total += len(b.Text) / 4
	}
	return total
}

func estimateMessageTokens(messages []anthropic.MessageParam) int {
	total := 0
	for _, msg := range messages {
		total += 4
		for _, block := range msg.Content {
			if block.OfText != nil {
				total += len(block.OfText.Text) / 4
			} else if block.OfToolUse != nil || block.OfToolResult != nil {
				total += 50
			}
		}
	}
	return total
}

// ListPromptBreakdowns lists prompt breakdown files for a session.
func ListPromptBreakdowns(logsDir, sessionID string) ([]string, error) {
	var files []string

	// New format: look in {logsDir}/*/{sessionID}/prompts/
	// Legacy: look in {logsDir}/*/prompts/{sessionID}-turn-*.md
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		// New format: turn-N.md inside a session dir
		if strings.HasPrefix(info.Name(), "turn-") && strings.Contains(path, sessionID) {
			files = append(files, path)
		}
		// Legacy format: sessionID-turn-N.md
		if strings.HasPrefix(info.Name(), sessionID) {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// ReadPromptBreakdown reads a specific prompt breakdown.
func ReadPromptBreakdown(logsDir, sessionID string, turn int) (string, error) {
	newFilename := fmt.Sprintf("turn-%d.md", turn)
	legacyFilename := fmt.Sprintf("%s-turn-%d.md", sessionID, turn)

	var found string
	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if (info.Name() == newFilename && strings.Contains(path, sessionID)) ||
			info.Name() == legacyFilename {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("prompt breakdown not found: %s turn %d", sessionID, turn)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// end of file
