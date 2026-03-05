package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// sanitizeToolID ensures a tool ID matches Anthropic's pattern ^[a-zA-Z0-9_-]+$
// OpenAI/GLM may generate IDs with dots, colons, or other characters.
func sanitizeToolID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "tool_" + fmt.Sprintf("%d", len(id))
	}
	return result
}

// LoadMessages reads a session JSONL and reconstructs the conversation as MessageParams.
// Correctly groups tool_use blocks into assistant messages and tool_result blocks into user messages.
func LoadMessages(logFile string) ([]anthropic.MessageParam, error) {
	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("open session log: %w", err)
	}
	defer f.Close()

	var messages []anthropic.MessageParam
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB lines

	// Collect entries first, then reconstruct properly
	var entries []Entry
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		switch entry.Role {
		case "user", "assistant", "tool_call", "tool_result":
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Reconstruct messages:
	// - user entries → user message
	// - assistant + following tool_call entries → single assistant message with text + tool_use blocks
	// - tool_result entries → single user message with tool_result blocks
	i := 0
	for i < len(entries) {
		e := entries[i]

		switch e.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(e.Content),
			))
			i++

		case "assistant":
			// Collect assistant text + any immediately following tool_calls
			var blocks []anthropic.ContentBlockParamUnion
			if e.Content != "" {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfText: &anthropic.TextBlockParam{Text: e.Content},
				})
			}
			i++
			// Absorb following tool_call entries into the same assistant message
			for i < len(entries) && entries[i].Role == "tool_call" {
				tc := entries[i]
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    sanitizeToolID(tc.ToolID),
						Name:  tc.ToolName,
						Input: json.RawMessage(tc.ToolInput),
					},
				})
				i++
			}
			messages = append(messages, anthropic.MessageParam{
				Role:    "assistant",
				Content: blocks,
			})

		case "tool_call":
			// Tool calls without a preceding assistant entry — create assistant message
			var blocks []anthropic.ContentBlockParamUnion
			for i < len(entries) && entries[i].Role == "tool_call" {
				tc := entries[i]
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    sanitizeToolID(tc.ToolID),
						Name:  tc.ToolName,
						Input: json.RawMessage(tc.ToolInput),
					},
				})
				i++
			}
			messages = append(messages, anthropic.MessageParam{
				Role:    "assistant",
				Content: blocks,
			})

		case "tool_result":
			// Collect consecutive tool_results into one user message
			var blocks []anthropic.ContentBlockParamUnion
			for i < len(entries) && entries[i].Role == "tool_result" {
				tr := entries[i]
				blocks = append(blocks, anthropic.NewToolResultBlock(
					sanitizeToolID(tr.ToolID), tr.Content, tr.IsError,
				))
				i++
			}
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		}
	}

	return messages, nil
}

// FindLatestSessionDir finds the most recent session directory for a given command.
// Returns the session directory path (containing session.jsonl, messages.json, etc).
func FindLatestSessionDir(logsDir, command string) (string, error) {
	searchDir := filepath.Join(logsDir, command)
	if _, err := os.Stat(searchDir); os.IsNotExist(err) {
		return "", fmt.Errorf("no sessions found in %s", searchDir)
	}

	type candidate struct {
		dir  string
		name string
	}
	var candidates []candidate

	filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "session.jsonl" {
			sessionDir := filepath.Dir(path)
			candidates = append(candidates, candidate{dir: sessionDir, name: filepath.Base(sessionDir)})
		} else if strings.HasSuffix(d.Name(), ".jsonl") {
			// Legacy flat format — dir is the parent
			candidates = append(candidates, candidate{dir: filepath.Dir(path), name: d.Name()})
		}
		return nil
	})

	if len(candidates) == 0 {
		return "", fmt.Errorf("no sessions found in %s", searchDir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].name > candidates[j].name
	})

	return candidates[0].dir, nil
}

// LoadMessagesFromDir loads conversation messages from a session directory.
// Prefers messages.json (exact snapshot) over reconstructing from JSONL.
func LoadMessagesFromDir(sessionDir string) ([]anthropic.MessageParam, error) {
	// Try messages.json first (exact snapshot)
	msgPath := filepath.Join(sessionDir, "messages.json")
	if data, err := os.ReadFile(msgPath); err == nil {
		var messages []anthropic.MessageParam
		if err := json.Unmarshal(data, &messages); err == nil {
			return messages, nil
		}
	}

	// Fall back to JSONL reconstruction
	jsonlPath := filepath.Join(sessionDir, "session.jsonl")
	return LoadMessages(jsonlPath)
}
