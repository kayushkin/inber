package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kayushkin/inber/session"
)

// configuredLogsRoots returns every distinct logs root the configured agents
// point at, in a stable order.
//
// A logs root belongs to a workspace, not to an agent. session.New writes
// <workspace>/logs/<agent>/<session>/session.jsonl and owns the <agent>
// segment itself, so the root is shared by every agent that runs there — and
// two agents do land in one workspace, because a workspace is a repo root and
// buildConfigFromRegistry resolves an agent with no explicit workspace to
// ~/life/repos/<project>, keyed on the agent's project rather than its name.
// Walking once per agent would therefore report every session in a shared root
// once per agent sharing it, each copy labelled with whichever agent the loop
// happened to be on.
func (g *Server) configuredLogsRoots() []string {
	seen := make(map[string]struct{}, len(g.config.Agents))
	roots := make([]string, 0, len(g.config.Agents))
	for _, ac := range g.config.Agents {
		if ac.Workspace == "" {
			continue
		}
		root := filepath.Join(ac.Workspace, "logs")
		if _, dup := seen[root]; dup {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

// agentFromSessionParent names the agent that owns a session, given the logs
// root it was found under and the directory holding the session.
//
// The agent is the FIRST path component below the root, never the whole
// relative path: session.New owns the layout and writes exactly one agent
// segment directly under the root. Sessions written before inber 6271cfa sit
// one level deeper — logs/<agent>/<agent>/<session> — because the caller joined
// the agent name a second time, and taking the first component reads those
// correctly instead of reporting the agent as "claxon/claxon", a name no agent
// has. A session written with no agent name sits directly under the root and
// has no agent to report.
func agentFromSessionParent(logsDir, sessionParent string) string {
	rel, err := filepath.Rel(logsDir, sessionParent)
	if err != nil || rel == "." {
		return ""
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	if first == ".." {
		return ""
	}
	return first
}

// GET /api/sessions/history?agent=<name>&limit=<n>
// Lists historical sessions from the logs directory.
//
// The agent filter matches the agent a session was logged under, which is not
// the same thing as the agent whose configuration named the workspace: one
// workspace can hold several agents' sessions, so filtering by the configured
// name would return every agent's sessions in that shared root.
func (g *Server) handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	agentFilter := r.URL.Query().Get("agent")

	type sessionEntry struct {
		ID        string    `json:"id"`
		Agent     string    `json:"agent"`
		StartTime time.Time `json:"start_time"`
		FilePath  string    `json:"file_path"`
	}

	var sessions []sessionEntry

	for _, logsDir := range g.configuredLogsRoots() {
		filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			var sessionID, agentName string
			switch {
			case d.Name() == "session.jsonl":
				// Format: logs/{agent}/{session_id}/session.jsonl
				sessionDir := filepath.Dir(path)
				sessionID = filepath.Base(sessionDir)
				agentName = agentFromSessionParent(logsDir, filepath.Dir(sessionDir))
			case strings.HasSuffix(d.Name(), ".jsonl") && d.Name() != "server-errors.jsonl":
				// Legacy: logs/{agent}/{session_id}.jsonl
				sessionID = strings.TrimSuffix(d.Name(), ".jsonl")
				agentName = agentFromSessionParent(logsDir, filepath.Dir(path))
			default:
				return nil
			}
			if agentFilter != "" && agentName != agentFilter {
				return nil
			}
			timePart := sessionID
			if len(timePart) > 17 {
				timePart = timePart[:17]
			}
			t, err := time.Parse("2006-01-02_150405", timePart)
			if err != nil {
				return nil
			}
			sessions = append(sessions, sessionEntry{ID: sessionID, Agent: agentName, StartTime: t, FilePath: path})
			return nil
		})
	}

	// Newest first, and by path within one timestamp: the session id carries
	// only whole seconds, so two sessions opened in the same second tie, and a
	// tie broken by nothing at all reorders the list between identical calls.
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].StartTime.Equal(sessions[j].StartTime) {
			return sessions[i].StartTime.After(sessions[j].StartTime)
		}
		return sessions[i].FilePath < sessions[j].FilePath
	})

	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	jsonResponse(w, sessions)
}

// GET /api/sessions/{key}/context — extract system prompt from session JSONL
func (g *Server) handleSessionContext(w http.ResponseWriter, r *http.Request, sessionID string) {
	logFile := g.findSessionLogFile(sessionID)
	if logFile == "" {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry session.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Role == "request" && len(entry.Request) > 0 {
			var req struct {
				System string `json:"system"`
			}
			if err := json.Unmarshal(entry.Request, &req); err == nil && req.System != "" {
				jsonResponse(w, map[string]string{"context": req.System})
				return
			}
		}
	}

	jsonResponse(w, map[string]string{"context": ""})
}

// GET /api/sessions/{key}/timeline — return session timeline
func (g *Server) handleSessionTimeline(w http.ResponseWriter, r *http.Request, sessionID string) {
	logsDir := g.findLogsDir(sessionID)
	if logsDir == "" {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	content, err := session.ReadTimelineFromJSONL(logsDir, sessionID, g.modelStore)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"timeline": content})
}

// GET /api/sessions/{key}/prompts — list prompt breakdowns
func (g *Server) handleSessionPrompts(w http.ResponseWriter, r *http.Request, sessionID string) {
	logsDir := g.findLogsDir(sessionID)
	if logsDir == "" {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	files, err := session.ListPromptBreakdowns(logsDir, sessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return just filenames, not full paths.
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	jsonResponse(w, names)
}

// GET /api/sessions/{key}/prompts/{turn} — read specific prompt breakdown
func (g *Server) handleSessionPromptDetail(w http.ResponseWriter, r *http.Request, sessionID string, turn int) {
	logsDir := g.findLogsDir(sessionID)
	if logsDir == "" {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	content, err := session.ReadPromptBreakdown(logsDir, sessionID, turn)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"content": content})
}

// findSessionLogFile searches all agent workspaces for a session's JSONL log file.
func (g *Server) findSessionLogFile(sessionID string) string {
	for _, ac := range g.config.Agents {
		if ac.Workspace == "" {
			continue
		}
		logsDir := filepath.Join(ac.Workspace, "logs")
		if f := findSessionFileInDir(logsDir, sessionID); f != "" {
			return f
		}
	}
	return ""
}

// findLogsDir returns the logs directory that contains the given session.
func (g *Server) findLogsDir(sessionID string) string {
	for _, ac := range g.config.Agents {
		if ac.Workspace == "" {
			continue
		}
		logsDir := filepath.Join(ac.Workspace, "logs")
		if f := findSessionFileInDir(logsDir, sessionID); f != "" {
			return logsDir
		}
	}
	return ""
}

// findSessionFileInDir searches a logs directory for a session file.
func findSessionFileInDir(logsDir, sessionID string) string {
	var logFile string
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "session.jsonl" && strings.Contains(filepath.Dir(path), sessionID) {
			logFile = path
			return filepath.SkipAll
		}
		if strings.Contains(d.Name(), sessionID) && strings.HasSuffix(d.Name(), ".jsonl") {
			logFile = path
			return filepath.SkipAll
		}
		return nil
	})
	return logFile
}
