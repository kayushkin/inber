package server

import "github.com/kayushkin/inber/agent"

// toolsForAgent returns the set of server-provided tools for an agent session.
// Orchestrators get workspace management tools; all agents get spawn/steer/list.
func (g *Server) toolsForAgent(sessionKey, agentName string) []agent.Tool {
	tools := []agent.Tool{
		g.SpawnAgentTool(sessionKey),
		g.SessionsListTool(sessionKey),
		g.SteerAgentTool(),
	}

	// Orchestrator agents get workspace management tools.
	if agentName == g.config.DefaultAgent {
		tools = append(tools,
			g.MergeWorkspaceTool(),
			g.RejectWorkspaceTool(),
			g.FixWorkspaceTool(),
			g.ListWorkspacesTool(),
		)
	}

	return tools
}