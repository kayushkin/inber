package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
)

// SessionWithRequests combines in-memory session info with recent request history.
type SessionWithRequests struct {
	*SessionInfo
	ActiveRequest  *RequestRow   `json:"active_request,omitempty"`
	RecentRequests []RequestRow  `json:"recent_requests,omitempty"`
}

// GET /api/sessions
func (g *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessions := g.ListSessions()

	// Enrich with DB data if requested.
	if r.URL.Query().Get("requests") == "true" {
		var enriched []SessionWithRequests
		for _, s := range sessions {
			swr := SessionWithRequests{SessionInfo: s}
			if active, _ := g.store.ActiveRequest(s.Key); active != nil {
				swr.ActiveRequest = active
			}
			if recent, _ := g.store.RecentRequests(s.Key, 5); len(recent) > 0 {
				swr.RecentRequests = recent
			}
			enriched = append(enriched, swr)
		}
		jsonResponse(w, enriched)
		return
	}

	jsonResponse(w, sessions)
}

// GET/DELETE /api/sessions/:key
func (g *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if key == "" {
		jsonError(w, "session key required", http.StatusBadRequest)
		return
	}

	// Handle messages endpoint.
	if strings.HasSuffix(key, "/messages") {
		key = strings.TrimSuffix(key, "/messages")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		g.handleSessionMessages(w, r, key)
		return
	}

	// Handle inject.
	if strings.HasSuffix(key, "/inject") {
		key = strings.TrimSuffix(key, "/inject")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Message string `json:"message"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := g.Inject(key, body.Message); err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, map[string]string{"status": "injected"})
		return
	}

	// Handle context.
	if strings.HasSuffix(key, "/context") {
		key = strings.TrimSuffix(key, "/context")
		g.handleSessionContext(w, r, key)
		return
	}

	// Handle timeline.
	if strings.HasSuffix(key, "/timeline") {
		key = strings.TrimSuffix(key, "/timeline")
		g.handleSessionTimeline(w, r, key)
		return
	}

	// Handle prompts — /prompts or /prompts/<turn>.
	if strings.Contains(key, "/prompts") {
		parts := strings.SplitN(key, "/prompts", 2)
		sessionKey := parts[0]
		rest := strings.TrimPrefix(parts[1], "/")
		if rest == "" {
			g.handleSessionPrompts(w, r, sessionKey)
		} else {
			turn := 0
			fmt.Sscanf(rest, "%d", &turn)
			g.handleSessionPromptDetail(w, r, sessionKey, turn)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		val, ok := g.sessions.Load(key)
		if !ok {
			jsonError(w, "session not found", http.StatusNotFound)
			return
		}
		s := val.(*Session)
		s.mu.Lock()
		info := &SessionInfo{
			Key:        s.Key,
			Agent:      s.AgentName,
			Status:     s.Status,
			SpawnDepth: s.SpawnDepth,
			ParentKey:  s.ParentKey,
			Children:   s.Children,
			CreatedAt:  s.CreatedAt,
			LastActive: s.LastActive,
			Messages:   len(s.Engine.Messages),
		}
		s.mu.Unlock()
		jsonResponse(w, info)

	case http.MethodDelete:
		if err := g.StopSession(key); err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, map[string]string{"status": "stopped"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/sessions/:key/messages — return conversation messages for dash history
func (g *Server) handleSessionMessages(w http.ResponseWriter, r *http.Request, key string) {
	type toolInfo struct {
		Tool   string `json:"tool"`
		Input  string `json:"input,omitempty"`
		Output string `json:"output,omitempty"`
	}
	type historyMsg struct {
		ID        string     `json:"id"`
		Role      string     `json:"role"`
		Content   string     `json:"content"`
		Agent     string     `json:"agent"`
		Timestamp string     `json:"timestamp"`
		Thinking  string     `json:"thinking,omitempty"`
		Tools     []toolInfo `json:"tools,omitempty"`
	}

	var msgs []anthropic.MessageParam

	if val, ok := g.sessions.Load(key); ok {
		s := val.(*Session)
		s.mu.Lock()
		msgs = make([]anthropic.MessageParam, len(s.Engine.Messages))
		copy(msgs, s.Engine.Messages)
		s.mu.Unlock()
	} else {
		// History rendering wants the transcript only; the turn count that
		// comes with it has no reader here.
		msgs, _ = g.loadPersistedSession(key)
	}

	if len(msgs) == 0 {
		jsonResponse(w, []historyMsg{})
		return
	}

	// Extract agent name from session key (agent:NAME:main)
	agentName := key
	parts := strings.SplitN(key, ":", 3)
	if len(parts) >= 2 {
		agentName = parts[1]
	}

	// Build a map of tool_use ID → name+input for pairing with results
	toolUseMap := make(map[string]toolInfo)

	var result []historyMsg
	for i, m := range msgs {
		var text, thinking string
		var tools []toolInfo
		for _, block := range m.Content {
			if block.OfText != nil {
				text += block.OfText.Text
			} else if block.OfThinking != nil {
				thinking += block.OfThinking.Thinking
			} else if block.OfToolUse != nil {
				ti := toolInfo{Tool: block.OfToolUse.Name, Input: conversation.ToolInputText(block.OfToolUse.Input)}
				tools = append(tools, ti)
				toolUseMap[block.OfToolUse.ID] = ti
			} else if block.OfToolResult != nil {
				// Extract output text from tool result content
				output := ""
				for _, c := range block.OfToolResult.Content {
					if c.OfText != nil {
						output += c.OfText.Text
					}
				}
				if tu, ok := toolUseMap[block.OfToolResult.ToolUseID]; ok {
					tu.Output = output
					tools = append(tools, tu)
				} else {
					tools = append(tools, toolInfo{Tool: "unknown", Output: output})
				}
			}
		}
		if text == "" && thinking == "" && len(tools) == 0 {
			continue
		}
		result = append(result, historyMsg{
			ID:        fmt.Sprintf("inber-%s-%d", key, i),
			Role:      string(m.Role),
			Content:   text,
			Agent:     agentName,
			Timestamp: time.Now().Add(-time.Duration(len(msgs)-i) * time.Minute).Format(time.RFC3339),
			Thinking:  thinking,
			Tools:     tools,
		})
	}

	jsonResponse(w, result)
}