package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/mcp"
)

// A session's MCP servers come from its caller. llm-bridge decides which MCP
// servers a session may have (tool-store's opt-ins for the instance) and hands
// them over in the shape Claude Code reads with --mcp-config. Claude Code
// starts each server itself, and the server inherits Claude Code's
// environment, which carries values that belong to that one session, such as
// the token it posts into its own chat with. inber-server is one long-running
// process serving every session, so it has none of those values: the caller
// sends them as mcp_environment, and inber starts the session's servers with
// them.
//
// Both are kept in memory only, by session key. The environment holds a
// session's credentials, which do not belong in the session database, and
// llm-bridge sends both again whenever it starts the session's harness, with
// a fresh token each time (it refuses the previous one). A session rebuilt
// after inber-server restarts has no MCP tools until its caller resumes it.

// mcpServerConfig is one server in a Claude Code MCP config. Only stdio
// servers can be started here: inber's MCP client speaks stdio.
type mcpServerConfig struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// sessionMCPConfig is what a session's caller sent: its servers and the
// environment values that belong to the session.
type sessionMCPConfig struct {
	Servers     map[string]mcpServerConfig
	Environment map[string]string
}

// parseSessionMCPConfig checks a caller's MCP servers. A server inber cannot
// start is refused here, when the caller sends it, rather than when the
// session's engine is built.
func parseSessionMCPConfig(servers json.RawMessage, environment map[string]string) (*sessionMCPConfig, error) {
	parsed := map[string]mcpServerConfig{}
	if err := json.Unmarshal(servers, &parsed); err != nil {
		return nil, fmt.Errorf("mcp_servers is not an object of MCP servers: %w", err)
	}
	for name, server := range parsed {
		if strings.Contains(name, "__") {
			return nil, fmt.Errorf("MCP server name %q contains \"__\", which separates server from tool in a tool's name", name)
		}
		if server.Type != "" && server.Type != "stdio" {
			return nil, fmt.Errorf("MCP server %q is type %q; inber can start only stdio servers", name, server.Type)
		}
		if server.Command == "" {
			return nil, fmt.Errorf("MCP server %q has no command", name)
		}
	}
	return &sessionMCPConfig{Servers: parsed, Environment: environment}, nil
}

// sessionMCPToolName is the name a session's model sees for an MCP tool. It is
// Claude Code's form, so the same tool has the same name in every harness the
// bridge drives, and two servers can offer a tool of the same name.
func sessionMCPToolName(server, tool string) string {
	return "mcp__" + server + "__" + tool
}

// sessionMCPClients is the running servers of one session. Its tools look the
// client up when they run, so replacing the clients (a resume with a fresh
// token) changes nothing in the engine.
type sessionMCPClients struct {
	mu      sync.Mutex
	clients map[string]*mcp.Client
}

// startSessionMCPServers starts every server in the config. If any fails, the
// ones already started are stopped and the error names the server.
func startSessionMCPServers(config *sessionMCPConfig) (map[string]*mcp.Client, error) {
	names := make([]string, 0, len(config.Servers))
	for name := range config.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	started := map[string]*mcp.Client{}
	for _, name := range names {
		server := config.Servers[name]
		environment := make([]string, 0, len(config.Environment)+len(server.Env))
		for key, value := range config.Environment {
			environment = append(environment, key+"="+value)
		}
		// The server's own env comes last, so it wins, as in Claude Code.
		for key, value := range server.Env {
			environment = append(environment, key+"="+value)
		}
		client, err := mcp.NewClientWithEnvironment(append([]string{server.Command}, server.Args...), environment)
		if err != nil {
			for _, running := range started {
				running.Close()
			}
			return nil, fmt.Errorf("start MCP server %q: %w", name, err)
		}
		started[name] = client
	}
	return started, nil
}

func newSessionMCPClients(config *sessionMCPConfig) (*sessionMCPClients, error) {
	clients, err := startSessionMCPServers(config)
	if err != nil {
		return nil, err
	}
	return &sessionMCPClients{clients: clients}, nil
}

// tools is every tool the session's servers offer, as inber tools, in a stable
// order: by server name, then in the order each server lists them.
func (c *sessionMCPClients) tools() []agent.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.clients))
	for name := range c.clients {
		names = append(names, name)
	}
	sort.Strings(names)

	var tools []agent.Tool
	for _, server := range names {
		for _, info := range c.clients[server].ListTools() {
			server, toolName := server, info.Name
			tools = append(tools, agent.Tool{
				Name:        sessionMCPToolName(server, toolName),
				Description: info.Description,
				InputSchema: info.InputSchema,
				Run: func(ctx context.Context, input string) (string, error) {
					return c.call(ctx, server, toolName, input)
				},
			})
		}
	}
	return tools
}

func (c *sessionMCPClients) call(ctx context.Context, server, tool, input string) (string, error) {
	c.mu.Lock()
	client := c.clients[server]
	c.mu.Unlock()
	if client == nil {
		return "", fmt.Errorf("MCP server %q is no longer running for this session", server)
	}
	return client.CallTool(ctx, tool, input)
}

// replace starts the servers in config and, once they are all running, stops
// the old ones. A server that fails to start leaves the old ones in place.
// A tool the engine was built with that the new servers no longer offer will
// fail when called, and the log says so now.
func (c *sessionMCPClients) replace(config *sessionMCPConfig) error {
	started, err := startSessionMCPServers(config)
	if err != nil {
		return err
	}
	c.mu.Lock()
	old := c.clients
	c.clients = started
	c.mu.Unlock()

	offered := map[string]bool{}
	for server, client := range started {
		for _, info := range client.ListTools() {
			offered[sessionMCPToolName(server, info.Name)] = true
		}
	}
	for server, client := range old {
		for _, info := range client.ListTools() {
			if name := sessionMCPToolName(server, info.Name); !offered[name] {
				log.Printf("[mcp] tool %s is gone from the restarted servers; calls to it will fail until the session is rebuilt", name)
			}
		}
		client.Close()
	}
	return nil
}

func (c *sessionMCPClients) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, client := range c.clients {
		client.Close()
	}
	c.clients = nil
}

// recordSessionMCPConfig keeps a session's MCP config, and restarts the
// servers of a live session with it.
func (g *Server) recordSessionMCPConfig(key string, config *sessionMCPConfig) error {
	g.mcpConfigs.Store(key, config)
	val, ok := g.sessions.Load(key)
	if !ok {
		return nil
	}
	s := val.(*Session)
	s.mu.Lock()
	running := s.mcp
	s.mu.Unlock()
	if running == nil {
		// The engine was built without MCP tools, and its tool set is fixed
		// when it is built.
		log.Printf("[mcp] session %s is live without MCP servers; the %d it was just given reach it when it is rebuilt", key, len(config.Servers))
		return nil
	}
	return running.replace(config)
}

// sessionMCPConfigFor is the config recorded for a session, or nil.
func (g *Server) sessionMCPConfigFor(key string) *sessionMCPConfig {
	val, ok := g.mcpConfigs.Load(key)
	if !ok {
		return nil
	}
	return val.(*sessionMCPConfig)
}
