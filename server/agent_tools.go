package server

import "github.com/kayushkin/inber/agent"

// toolsForAgent returns the set of server-provided tools for an agent session.
// Orchestrators get workspace management tools; all agents get spawn/steer/list.
func (g *Server) toolsForAgent(sessionKey, agentName string) []agent.Tool {
	isOrchestrator := agentName == g.config.DefaultAgent

	tools := []agent.Tool{
		g.SpawnAgentTool(sessionKey),
		g.SteerAgentTool(),
	}

	// Non-orchestrator agents get sessions_list and agents_status as tools.
	// The orchestrator gets these injected into its prompt via ContextInjectors instead.
	if !isOrchestrator {
		tools = append(tools,
			g.SessionsListTool(sessionKey),
			g.AgentsStatusTool(),
		)
	}

	// Orchestrator agents get workspace management tools.
	if isOrchestrator {
		tools = append(tools,
			g.MergeWorkspaceTool(),
			g.RejectWorkspaceTool(),
			g.FixWorkspaceTool(),
			g.ListWorkspacesTool(),
		)
	}

	return tools
}