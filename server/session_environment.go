package server

import "sort"

// A session's environment is the NAME=value entries its caller sent that
// belong to that one session. llm-bridge gives every harness process values
// such as the token it posts into its own chat with and the routes it posts
// to (LLM_BRIDGE_SESSION_*); a harness it starts per session inherits them,
// and a session's shell commands and MCP servers inherit them from it.
// inber-server is one process serving every session, so the caller sends
// them, and inber hands them to that session's shell commands
// (engine.EngineConfig.SessionEnvironment) and MCP servers (session_mcp.go).
//
// They are kept in memory only, by session key: they are the session's
// credentials, which do not belong in the session database, and llm-bridge
// sends them again whenever it starts the session's harness, with a fresh
// token each time.

func (g *Server) recordSessionEnvironment(key string, environment map[string]string) {
	g.sessionEnvironments.Store(key, environment)
}

// sessionEnvironmentEntries is the session's environment as NAME=value
// entries in a stable order, or nil for a session that has none. It reads the
// current values, so a caller that keeps the function rather than the
// entries sees a resume's refresh.
func (g *Server) sessionEnvironmentEntries(key string) []string {
	val, ok := g.sessionEnvironments.Load(key)
	if !ok {
		return nil
	}
	environment := val.(map[string]string)
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, name+"="+environment[name])
	}
	return entries
}
