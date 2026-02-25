package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// PromptBreakdown writes a prompt breakdown file for a given turn.
func WritePromptBreakdown(logFilePath string, sessionID string, turn int, params *anthropic.MessageNewParams) error {
	// Derive prompts dir from log path
	logsDir := filepath.Dir(logFilePath)
	promptsDir := filepath.Join(logsDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}

	filename := fmt.Sprintf("%s-turn-%d.md", sessionID, turn)
	path := filepath.Join(promptsDir, filename)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Prompt Breakdown — Turn %d\n\n", turn))
	sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Model:** %s\n\n", params.Model))

	// System prompt
	systemTokens := 0
	if len(params.System) > 0 {
		sb.WriteString("## System Prompt\n\n")
		for i, block := range params.System {
			text := block.Text
			tokens := len(text) / 4
			systemTokens += tokens
			sb.WriteString(fmt.Sprintf("### Block %d (%d tokens)\n\n", i+1, tokens))
			if len(text) > 500 {
				sb.WriteString(text[:500] + "...\n\n")
			} else {
				sb.WriteString(text + "\n\n")
			}
		}
	}

	// Messages
	messageTokens := 0
	sb.WriteString("## Message History\n\n")
	sb.WriteString("| # | Role | Tokens (est) | Content |\n")
	sb.WriteString("|---|------|-------------|--------|\n")
	for i, msg := range params.Messages {
		role := string(msg.Role)
		msgTokens := 4 // overhead
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
		messageTokens += msgTokens
		contentPreview = strings.ReplaceAll(contentPreview, "\n", " ")
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %s |\n", i+1, role, msgTokens, contentPreview))
	}

	// Tool definitions
	toolTokens := len(params.Tools) * 100 // rough estimate per tool
	sb.WriteString(fmt.Sprintf("\n## Token Breakdown\n\n"))
	sb.WriteString(fmt.Sprintf("- **System prompt:** ~%d tokens\n", systemTokens))
	sb.WriteString(fmt.Sprintf("- **Messages:** ~%d tokens (%d messages)\n", messageTokens, len(params.Messages)))
	sb.WriteString(fmt.Sprintf("- **Tool definitions:** ~%d tokens (%d tools)\n", toolTokens, len(params.Tools)))
	sb.WriteString(fmt.Sprintf("- **Estimated total:** ~%d tokens\n", systemTokens+messageTokens+toolTokens))

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// ListPromptBreakdowns lists prompt breakdown files for a session.
func ListPromptBreakdowns(logsDir, sessionID string) ([]string, error) {
	// Search for prompts dirs in logsDir and subdirs
	var files []string

	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() != "prompts" && info.Name() != logsDir && path != logsDir {
			// Check for prompts subdir
			promptsDir := filepath.Join(path, "prompts")
			if _, err := os.Stat(promptsDir); err != nil {
				return nil
			}
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), sessionID) && strings.HasSuffix(info.Name(), ".md") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// ReadPromptBreakdown reads a specific prompt breakdown.
func ReadPromptBreakdown(logsDir, sessionID string, turn int) (string, error) {
	filename := fmt.Sprintf("%s-turn-%d.md", sessionID, turn)

	var found string
	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == filename {
			found = path
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
