package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kayushkin/inber/engine"
)

// fakeMCPServerEnvironmentVariable makes this test binary act as a stdio MCP
// server with one tool, read_environment, that returns the value of the
// environment variable its input names. A real process is the point: what the
// session's caller sends has to reach a server that inber starts.
const fakeMCPServerEnvironmentVariable = "INBER_TEST_ACT_AS_MCP_SERVER"

func runFakeMCPServer() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Arguments struct {
					Name string `json:"name"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fake"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "read_environment", "description": "Returns an environment variable.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
			}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": os.Getenv(request.Params.Arguments.Name)}}}
		}
		reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		fmt.Println(string(reply))
	}
}

// fakeMCPServers is the mcp_servers a caller would send to run the fake.
func fakeMCPServers(t *testing.T) json.RawMessage {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := json.Marshal(map[string]any{
		"fake": map[string]any{"command": executable, "env": map[string]string{fakeMCPServerEnvironmentVariable: "1"}},
	})
	return servers
}

func callReadEnvironment(t *testing.T, clients *sessionMCPClients, name string) string {
	t.Helper()
	for _, tool := range clients.tools() {
		if tool.Name == "mcp__fake__read_environment" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := tool.Run(ctx, fmt.Sprintf(`{"name":%q}`, name))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			return out
		}
	}
	t.Fatalf("no mcp__fake__read_environment among %d tools", len(clients.tools()))
	return ""
}

func TestASessionsMCPServerRunsWithTheEnvironmentItsCallerSent(t *testing.T) {
	config, err := parseSessionMCPConfig(fakeMCPServers(t))
	if err != nil {
		t.Fatal(err)
	}
	clients, err := newSessionMCPClients(config, []string{"LLM_BRIDGE_SESSION_POSTING_TOKEN=spawn-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer clients.close()

	if got := callReadEnvironment(t, clients, "LLM_BRIDGE_SESSION_POSTING_TOKEN"); got != "spawn-1" {
		t.Fatalf("the server saw token %q, want spawn-1", got)
	}

	// A resume brings a fresh token, and the same tool reaches it without the
	// engine's tool list changing.
	next, err := parseSessionMCPConfig(fakeMCPServers(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := clients.replace(next, []string{"LLM_BRIDGE_SESSION_POSTING_TOKEN=spawn-2"}); err != nil {
		t.Fatal(err)
	}
	if got := callReadEnvironment(t, clients, "LLM_BRIDGE_SESSION_POSTING_TOKEN"); got != "spawn-2" {
		t.Fatalf("after the resume the server saw token %q, want spawn-2", got)
	}
}

func TestMCPServersInberCannotStartAreRefusedWhenSent(t *testing.T) {
	for name, servers := range map[string]string{
		"http server":    `{"remote":{"type":"http","url":"https://example.invalid/mcp"}}`,
		"no command":     `{"empty":{}}`,
		"__ in the name": `{"a__b":{"command":"x"}}`,
		"not an object":  `["x"]`,
	} {
		if _, err := parseSessionMCPConfig(json.RawMessage(servers)); err == nil {
			t.Errorf("%s: accepted %s", name, servers)
		}
	}
}

func TestTheBridgeRoutesKeepAndRestartASessionsMCPServers(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DefaultAgent: "claxon", Agents: map[string]AgentConfig{"claxon": {}}}}

	body, _ := json.Marshal(map[string]any{"harness": "inber", "agent_id": "claxon",
		"mcp_servers": fakeMCPServers(t), "session_environment": map[string]string{"LLM_BRIDGE_SESSION_POSTING_TOKEN": "spawn-1"}})
	recorder := httptest.NewRecorder()
	server.handleBridgeSessions(recorder, httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(string(body))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	var created bridgeSession
	json.Unmarshal(recorder.Body.Bytes(), &created)
	if config := server.sessionMCPConfigFor(created.SessionID); config == nil || len(config.Servers) != 1 {
		t.Fatalf("recorded config = %+v", config)
	}
	if got := server.sessionEnvironmentEntries(created.SessionID); len(got) != 1 || got[0] != "LLM_BRIDGE_SESSION_POSTING_TOKEN=spawn-1" {
		t.Fatalf("recorded environment = %v", got)
	}

	// A live session's servers restart with the resume's environment.
	initial, _ := parseSessionMCPConfig(fakeMCPServers(t))
	clients, err := newSessionMCPClients(initial, server.sessionEnvironmentEntries(created.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{Key: created.SessionID, AgentName: "claxon", Engine: &engine.Engine{}, mcp: clients}
	server.sessions.Store(created.SessionID, session)
	defer session.close()

	body, _ = json.Marshal(map[string]any{"mcp_servers": fakeMCPServers(t), "session_environment": map[string]string{"LLM_BRIDGE_SESSION_POSTING_TOKEN": "spawn-2"}})
	recorder = httptest.NewRecorder()
	server.handleBridgeResume(recorder, httptest.NewRequest(http.MethodPost, "/sessions/"+created.SessionID+"/resume", strings.NewReader(string(body))), created.SessionID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := callReadEnvironment(t, clients, "LLM_BRIDGE_SESSION_POSTING_TOKEN"); got != "spawn-2" {
		t.Fatalf("after the resume the live session's server saw %q, want spawn-2", got)
	}

	// A create naming a server inber cannot start is refused.
	recorder = httptest.NewRecorder()
	server.handleBridgeSessions(recorder, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"harness":"inber","agent_id":"claxon","mcp_servers":{"remote":{"type":"http","url":"https://example.invalid"}}}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("create with an http server: %d, want 400", recorder.Code)
	}
}

// The servers start with the engine and their tools reach it. Building an
// engine needs the host's agent configuration, so this skips where it cannot
// run, as the other createSession tests do.
func TestASessionIsBuiltWithItsMCPServersTools(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	const key = "agent:claxon:bridge-mcp"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}
	config, err := parseSessionMCPConfig(fakeMCPServers(t))
	if err != nil {
		t.Fatal(err)
	}
	server.mcpConfigs.Store(key, config)
	server.recordSessionEnvironment(key, map[string]string{"LLM_BRIDGE_SESSION_POSTING_TOKEN": "spawn-1"})

	created, err := server.createSession(context.Background(), key, "claxon", AgentConfig{Workspace: t.TempDir()}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}
	defer created.close()
	found := false
	for _, name := range created.Engine.EnabledToolNames() {
		if name == "mcp__fake__read_environment" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the engine's tools %v lack mcp__fake__read_environment", created.Engine.EnabledToolNames())
	}
	if created.mcp == nil {
		t.Fatal("the session does not hold its MCP clients, so closing it would leave the server running")
	}
}

// A session's shell commands get its environment too, read at each command,
// so bridge-share-file and bridge-button can reach the session's own chat. A
// resume's fresh token reaches the next command of a live session.
func TestASessionsShellCommandsRunWithItsEnvironment(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	const key = "agent:claxon:bridge-shell"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}
	server.recordSessionEnvironment(key, map[string]string{"LLM_BRIDGE_SESSION_POSTING_TOKEN": "spawn-1"})

	created, err := server.createSession(context.Background(), key, "claxon", AgentConfig{Workspace: t.TempDir()}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}
	defer created.close()
	shell := created.Engine.ToolByName("shell_commands")
	if shell == nil {
		t.Skip("the agent here has no shell_commands")
	}
	run := func() string {
		out, err := shell.Run(context.Background(), `{"command":"printf %s \"$LLM_BRIDGE_SESSION_POSTING_TOKEN\""}`)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := run(); got != "spawn-1" {
		t.Fatalf("the session's shell saw %q, want spawn-1", got)
	}
	server.recordSessionEnvironment(key, map[string]string{"LLM_BRIDGE_SESSION_POSTING_TOKEN": "spawn-2"})
	if got := run(); got != "spawn-2" {
		t.Fatalf("after a resume the session's shell saw %q, want spawn-2", got)
	}
}
