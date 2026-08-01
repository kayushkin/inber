package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Workspace is a generated, read-only view of a session's prompt, on disk for a
// human to inspect between turns.
//
// It is NOT a control surface. system/ is deleted and rewritten from the engine's
// own blocks before every turn, so an edit made to a file in it is discarded at the
// start of the next turn, and a file added to it is deleted. Nothing reads system/
// back — this comment used to promise that the engine did, and it never has.
//
// Making it editable is a design change, not a missing call. The directory is keyed
// on the agent name, not the session, so two concurrent sessions of the same agent
// in the same repository share it; a read-back that asks "did a human change this
// file?" would have to distinguish a user's edit from a peer session's write, and
// answering that needs a per-write manifest and a decision about the sharing. See
// noteboard todo 9def7d6b and docs/write-read-gate-audit.md.
//
// messages.json is the exception and is genuinely read back — it is how a resumed
// session recovers its transcript (LoadMessages).
//
// Layout, under the repository root:
//
//	.inber/workspace/{agent}/
//	├── system/
//	│   ├── 01-memory-instructions.md
//	│   ├── 02-identity.md
//	│   ├── 03-repo-map.md
//	│   └── ...
//	├── tools.md
//	└── messages.json
type Workspace struct {
	Dir string // e.g. "<repo>/.inber/workspace/main"
}

// NewWorkspace creates a workspace for the given agent under baseDir/.inber/workspace/{agent}.
func NewWorkspace(baseDir, agentName string) *Workspace {
	dir := filepath.Join(baseDir, ".inber", "workspace", agentName)
	return &Workspace{Dir: dir}
}

// WriteSystem regenerates system/ from the given blocks, deleting whatever was
// there. Anything a user put in that directory, edit or new file, is lost here;
// see the type comment for why that is the behaviour and not a bug.
func (w *Workspace) WriteSystem(blocks []NamedBlock) error {
	sysDir := filepath.Join(w.Dir, "system")
	// Remove old system dir and recreate
	os.RemoveAll(sysDir)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		return fmt.Errorf("failed to create system directory %s: %w", sysDir, err)
	}

	for i, block := range blocks {
		slug := slugify(block.ID)
		if slug == "" {
			slug = fmt.Sprintf("block-%d", i+1)
		}
		filename := fmt.Sprintf("%02d-%s.md", i+1, slug)
		path := filepath.Join(sysDir, filename)
		if err := os.WriteFile(path, []byte(block.Text), 0644); err != nil {
			return fmt.Errorf("failed to write system block file %s: %w", path, err)
		}
	}
	return nil
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

// ClearMessages removes messages.json from the workspace, and the turn count
// that was recorded against it. The count is only meaningful as a description
// of that transcript, so leaving it behind would tell the next session it is
// fifty turns into a conversation with no messages in it.
func (w *Workspace) ClearMessages() {
	os.Remove(filepath.Join(w.Dir, "messages.json"))
	ClearTurnCounter(w.Dir)
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
