package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kayushkin/inber/session"
)

// GET /api/sessions/history?agent=<name>&limit=<n>
// Lists historical sessions from the logs directory.
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

	// Scan all agent workspaces for logs.
	for name, ac := range g.config.Agents {
		if agentFilter != "" && name != agentFilter {
			continue
		}
		if ac.Workspace == "" {
			continue
		}

		logsDir := filepath.Join(ac.Workspace, "logs")
		filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Name() == "session.jsonl" {
				// Format: logs/{agent}/{session_id}/session.jsonl
				sessionDir := filepath.Dir(path)
				sessionID := filepath.Base(sessionDir)
				agentDir := filepath.Dir(sessionDir)
				relAgent, _ := filepath.Rel(logsDir, agentDir)
				agentName := ""
				if relAgent != "." {
					agentName = relAgent
				}
				timePart := sessionID
				if len(timePart) > 17 {
					timePart = timePart[:17]
				}
				if t, err := time.Parse("2006-01-02_150405", timePart); err == nil {
					sessions = append(sessions, sessionEntry{ID: sessionID, Agent: agentName, StartTime: t, FilePath: path})
				}
			} else if strings.HasSuffix(d.Name(), ".jsonl") && d.Name() != "server-errors.jsonl" {
				// Legacy: logs/{agent}/{session_id}.jsonl
				relDir, _ := filepath.Rel(logsDir, filepath.Dir(path))
				agentName := ""
				if relDir != "." {
					agentName = relDir
				}
				n := strings.TrimSuffix(d.Name(), ".jsonl")
				if t, err := time.Parse("2006-01-02_150405", n); err == nil {
					sessions = append(sessions, sessionEntry{ID: n, Agent: agentName, StartTime: t, FilePath: path})
				}
			}
			return nil
		})
	}

	// Sort newest first.
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].StartTime.After(sessions[i].StartTime) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

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

	content, err := session.ReadTimelineFromJSONL(logsDir, sessionID)
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
