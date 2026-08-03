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

	// Non-orchestrator agents get agents_status as a tool.
	// The orchestrator gets this injected into its prompt via ContextInjectors instead.
	if !isOrchestrator {
		tools = append(tools,
			g.AgentsStatusTool(),
		)
	}

	// Orchestrator agents get workspace management tools.
	if isOrchestrator {
		tools = append(tools,
			g.MergeWorkspaceTool(),
			g.RejectWorkspaceTool(),
			g.FixWorkspaceTool(sessionKey),
			g.ListWorkspacesTool(),
		)
	}

	return tools
}