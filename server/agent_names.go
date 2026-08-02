package server

import "sort"

// sortedAgentNames returns the configured agent names in a stable order.
//
// Ranging a Go map yields a fresh permutation every time, so any output built
// that way differs run to run and call to call for no reason the reader can
// see. Three things in this package need the order to be stable and none of
// them can get it from the map:
//
//   - the spawn_agent tool description, which sits in the tools block that
//     carries the cache_control breakpoint (agent/agent_run.go), so a reordered
//     list costs a cache miss on the entire prefix;
//   - GET /api/agents, whose body the CLI's own spawn tool turns straight back
//     into a tool description (agent/registry/spawn_tool.go);
//   - the default-agent fallback, which decides which session is the
//     orchestrator and therefore which one holds the workspace merge tools.
//
// One function so the three cannot drift apart.
func sortedAgentNames(agents map[string]AgentConfig) []string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveDefaultAgent fills in Config.DefaultAgent when nothing configured one,
// and reports whether it had to guess so the caller can say so.
//
// A configured default is left exactly as it is. The guess is the first agent by
// name — which is a guess, not an answer, but it is at least the same guess on
// every restart. It used to be whichever key a map range happened to yield
// first, and agent_tools.go reads this field to decide which session is the
// orchestrator: the one that gets merge_workspace, reject_workspace and
// fix_workspace. Those rebase, merge, push to origin and delete worktrees, so
// which agent holds them is not a thing to draw from a hat at boot.
func resolveDefaultAgent(cfg *Config) (guessed bool) {
	if cfg.DefaultAgent != "" || len(cfg.Agents) == 0 {
		return false
	}
	cfg.DefaultAgent = sortedAgentNames(cfg.Agents)[0]
	return true
}
