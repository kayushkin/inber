package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kayushkin/inber/session"
)

// sessionLogsRoots returns every distinct logs root a session on this host can
// have written to, in a stable order.
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
//
// There are two sources for that set and both are needed. The agents' configured
// workspaces are where an ordinary session works. A session spawned into a forge
// workspace does not: workspaceRootsForSession hands the engine the worktree
// slot under ~/forge/work/<id>/<repo>, and the transcript is written there. The
// endpoint used to build its search set from configuration alone, so every one
// of those sessions was not merely missing from the listing — it was reported as
// not existing.
//
// The recorded workspaces come from the store rather than from a scan of forge's
// directories, because a reader that walks ~/forge/work/*/*/logs is a second
// place that knows where forge puts things, and it would go wrong the moment
// forge changed its layout. They are added to the configured roots rather than
// replacing them: the walk stays a filesystem scan, so a session whose store row
// is gone is still listed from disk exactly as it is today.
func (g *Server) sessionLogsRoots() ([]string, error) {
	seen := make(map[string]struct{}, len(g.config.Agents))
	roots := make([]string, 0, len(g.config.Agents))
	add := func(workspace string) {
		if workspace == "" {
			return
		}
		root := session.LogsRoot(workspace)
		if _, dup := seen[root]; dup {
			return
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}

	for _, ac := range g.config.Agents {
		add(ac.Workspace)
	}

	if g.store != nil {
		// A store that cannot be read is reported, never skipped. Answering
		// with the configured roots alone would be this defect again with the
		// failure hidden: a 200 listing that silently omits every session in a
		// workspace reads as "those sessions do not exist".
		recorded, err := g.store.RecordedPrimaryWorkspaces()
		if err != nil {
			return nil, err
		}
		for _, workspace := range recorded {
			add(workspace)
		}
	}

	sort.Strings(roots)
	return roots, nil
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

	logsRoots, err := g.sessionLogsRoots()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, logsDir := range logsRoots {
		filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			sessionID := session.SessionIDOfTranscript(path)
			if sessionID == "" {
				return nil
			}
			// The agent segment sits above the session directory in the current
			// layout and above the file itself in the legacy one, which is the
			// one thing the two layouts do not share.
			agentParent := filepath.Dir(path)
			if d.Name() == "session.jsonl" {
				agentParent = filepath.Dir(agentParent)
			}
			agentName := agentFromSessionParent(logsDir, agentParent)
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
	logFile, err := g.findSessionLogFile(sessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	logsDir, err := g.findLogsDir(sessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	logsDir, err := g.findLogsDir(sessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	logsDir, err := g.findLogsDir(sessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

// findSessionLogFile searches every logs root a session can have been written
// to for one session's JSONL log file, and returns an empty path when no root
// holds it.
//
// It searches the same set the listing walks, and for the same reason: a
// session in a forge workspace is under a root no agent's configuration names,
// so deriving the set here from configuration alone answered "not found" for
// every session the listing could not see either.
func (g *Server) findSessionLogFile(sessionID string) (string, error) {
	_, logFile, err := g.locateSession(sessionID)
	return logFile, err
}

// findLogsDir returns the logs root that contains the given session, and an
// empty path when no root holds it.
func (g *Server) findLogsDir(sessionID string) (string, error) {
	logsDir, _, err := g.locateSession(sessionID)
	return logsDir, err
}

// locateSession finds the one transcript a session id names, and returns the
// root holding it alongside it. Both are empty when no root holds the session.
//
// It searches every root rather than stopping at the first that answers, and
// refuses to choose when two answer. Session ids are minted per workspace —
// <timestamp>_<4 hex>, from a clock and a random suffix, with nothing in them
// naming the root — so two roots CAN hold the same id, and the old search
// resolved that by taking whichever root sorted first. That is a coin flip
// dressed as an answer: /context, /timeline and /prompts all read through here,
// so the loser's transcript is served under the winner's id with no error
// anywhere. An id that names two sessions is a broken question, and the caller
// is told so.
//
// Measured on this host 2026-08-02: 171 sessions across five roots, zero
// duplicate ids and zero ids that are a prefix of another. The refusal costs
// nothing today; it is what keeps the silent pick from coming back.
func (g *Server) locateSession(sessionID string) (logsDir, logFile string, err error) {
	roots, err := g.sessionLogsRoots()
	if err != nil {
		return "", "", err
	}
	for _, root := range roots {
		found := findSessionFilesInDir(root, sessionID)
		if len(found) == 0 {
			continue
		}
		if len(found) > 1 {
			return "", "", fmt.Errorf("session %s names %d transcripts under %s: %s", sessionID, len(found), root, strings.Join(found, ", "))
		}
		if logFile != "" {
			return "", "", fmt.Errorf("session %s names transcripts in two logs roots: %s and %s", sessionID, logFile, found[0])
		}
		logsDir, logFile = root, found[0]
	}
	return logsDir, logFile, nil
}

// findSessionFilesInDir returns every transcript in a logs directory that
// belongs to the given session, in walk order.
//
// It returns all of them because finding a second one is the only way to know
// the first was not the answer. The search this replaced returned the first hit
// and stopped, so it could not tell a unique match from an arbitrary one.
func findSessionFilesInDir(logsDir, sessionID string) []string {
	if sessionID == "" {
		// Every path that is not a transcript answers with the empty id, so an
		// empty query would otherwise match all of them.
		return nil
	}
	var found []string
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if session.SessionIDOfTranscript(path) == sessionID {
			found = append(found, path)
		}
		return nil
	})
	return found
}
