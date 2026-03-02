package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContextManagementLogging(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := New(tmpDir, "claude-sonnet-4-5-20250929", "test-agent", "")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer s.Close()

	// Test summarization logging
	s.LogSummarize(15, 250, 10, "mem_abc123")

	// Test pruning logging
	s.LogPrune(5, 1200, "role-based-truncation")

	// Test stashing logging
	s.LogStash("user", 2, 800)
	s.LogStash("assistant", 1, 600)

	// Test compaction logging (already exists)
	s.LogCompaction([]string{"mem_1", "mem_2"}, "mem_combined", []string{"tag1", "tag2"})

	s.Close()

	// Read the session log and verify entries
	logPath := filepath.Join(tmpDir, "test-agent", s.SessionID(), "session.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := 0
	summarizeFound := false
	pruneFound := false
	stashUserFound := false
	stashAssistantFound := false
	compactionFound := false

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines++
		}
	}

	// Parse each line
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse log entry: %v", err)
		}

		switch entry.Role {
		case "summarize":
			summarizeFound = true
			if entry.Content == "" {
				t.Error("summarize entry has empty content")
			}
		case "prune":
			pruneFound = true
			if entry.Content == "" {
				t.Error("prune entry has empty content")
			}
		case "stash":
			if entry.Content == "" {
				t.Error("stash entry has empty content")
			}
			// Parse the data to check message type
			var data map[string]interface{}
			if err := json.Unmarshal(entry.Request, &data); err != nil {
				t.Errorf("failed to parse stash data: %v", err)
			}
			if msgType, ok := data["message_type"].(string); ok {
				if msgType == "user" {
					stashUserFound = true
				} else if msgType == "assistant" {
					stashAssistantFound = true
				}
			}
		case "compaction":
			compactionFound = true
			if entry.Content == "" {
				t.Error("compaction entry has empty content")
			}
		}
	}

	if !summarizeFound {
		t.Error("summarize entry not found")
	}
	if !pruneFound {
		t.Error("prune entry not found")
	}
	if !stashUserFound {
		t.Error("stash (user) entry not found")
	}
	if !stashAssistantFound {
		t.Error("stash (assistant) entry not found")
	}
	if !compactionFound {
		t.Error("compaction entry not found")
	}

	t.Logf("Found %d log entries", lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
