package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Workspace is a live-editable prompt directory for a session.
// Before each turn, the engine writes the current prompt pieces here.
// The user can edit/delete files between turns, and the engine reads them back.
//
// Layout:
//
//	session/{agent}/
//	├── system/
//	│   ├── 01-memory-instructions.md
//	│   ├── 02-identity.md
//	│   ├── 03-repo-map.md
//	│   └── ...
//	├── tools.json
//	└── messages.json
type Workspace struct {
	Dir string // e.g. "session/main"
}

// NewWorkspace creates a workspace for the given agent under baseDir/.inber/workspace/{agent}.
func NewWorkspace(baseDir, agentName string) *Workspace {
	dir := filepath.Join(baseDir, ".inber", "workspace", agentName)
	return &Workspace{Dir: dir}
}

// WriteSystem writes each system prompt block as a numbered .md file.
// Clears old system files first so deleted blocks don't persist.
func (w *Workspace) WriteSystem(blocks []NamedBlock) error {
	sysDir := filepath.Join(w.Dir, "system")
	// Remove old system dir and recreate
	os.RemoveAll(sysDir)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		return err
	}

	for i, block := range blocks {
		slug := slugify(block.ID)
		if slug == "" {
			slug = fmt.Sprintf("block-%d", i+1)
		}
		filename := fmt.Sprintf("%02d-%s.md", i+1, slug)
		path := filepath.Join(sysDir, filename)
		if err := os.WriteFile(path, []byte(block.Text), 0644); err != nil {
			return err
		}
	}
	return nil
}

// ReadSystem reads system prompt blocks from the workspace.
// Files are sorted by name (so 01-*, 02-*, etc. maintain order).
// Returns nil if the system/ dir doesn't exist (first turn or deleted).
func (w *Workspace) ReadSystem() ([]NamedBlock, error) {
	sysDir := filepath.Join(w.Dir, "system")
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Sort by name for stable ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var blocks []NamedBlock
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sysDir, entry.Name()))
		if err != nil {
			continue
		}
		// Derive ID from filename: "02-identity.md" -> "identity"
		name := strings.TrimSuffix(entry.Name(), ".md")
		if idx := strings.Index(name, "-"); idx >= 0 {
			name = name[idx+1:]
		}
		blocks = append(blocks, NamedBlock{ID: name, Text: string(data)})
	}
	return blocks, nil
}

// ToolInfo is a minimal tool representation for workspace display.
type ToolInfo struct {
	Name        string
	Description string
}

// WriteToolsList writes the tool list to the workspace for reference.
func (w *Workspace) WriteToolsList(tools []ToolInfo) error {
	if err := os.MkdirAll(w.Dir, 0755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Tools\n\n")
	sb.WriteString(fmt.Sprintf("%d tools available\n\n", len(tools)))
	for i, t := range tools {
		sb.WriteString(fmt.Sprintf("## %d. %s\n", i+1, t.Name))
		if t.Description != "" {
			sb.WriteString(t.Description + "\n")
		}
		sb.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(w.Dir, "tools.md"), []byte(sb.String()), 0644)
}

// LoadMessages loads the persistent session messages from the workspace.
func (w *Workspace) LoadMessages() ([]anthropic.MessageParam, error) {
	data, err := os.ReadFile(filepath.Join(w.Dir, "messages.json"))
	if err != nil {
		return nil, err
	}
	var msgs []anthropic.MessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// SaveMessages writes messages.json to the workspace.
func (w *Workspace) SaveMessages(data []byte) error {
	if err := os.MkdirAll(w.Dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.Dir, "messages.json"), data, 0644)
}

// ClearMessages removes messages.json from the workspace.
func (w *Workspace) ClearMessages() {
	os.Remove(filepath.Join(w.Dir, "messages.json"))
}

// Exists returns true if the workspace directory exists.
func (w *Workspace) Exists() bool {
	_, err := os.Stat(w.Dir)
	return err == nil
}

// Clean removes the workspace directory.
func (w *Workspace) Clean() error {
	return os.RemoveAll(w.Dir)
}
