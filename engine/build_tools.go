package engine

import (
	"strings"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
)

// buildTools resolves tools from agent config or defaults.
//
// It does not give them the session's root directory: setToolSet does, on the
// whole installed set, because the server can inject tools that never pass
// through here and can replace a built-in one by name.
func (e *Engine) buildTools() []agent.Tool {
	if e.AgentConfig != nil && len(e.AgentConfig.Tools) > 0 {
		return e.buildConfiguredTools()
	}
	return e.buildDefaultTools()
}

// buildConfiguredTools builds tools from the agent config.
func (e *Engine) buildConfiguredTools() []agent.Tool {
	var result []agent.Tool
	
	for _, toolName := range e.AgentConfig.Tools {
		if tool := e.buildSpecialTool(toolName); tool != nil {
			result = append(result, *tool)
			continue
		}
		
		if tool := e.findStandardTool(toolName); tool != nil {
			result = append(result, *tool)
			continue
		}
	}
	
	// Add memory tools
	if e.MemStore != nil {
		result = append(result, e.buildMemoryTools()...)
	}
	
	return result
}

// buildDefaultTools returns all available tools with workspace adaptations.
func (e *Engine) buildDefaultTools() []agent.Tool {
	result := tools.All()

	// Add memory tools
	if e.MemStore != nil {
		result = append(result, memory.AllMemoryTools(e.MemStore)...)
	}
	
	// Add workspace tools
	ignorePatterns := []string{
		"*.log", "*.tmp", ".git/*", "vendor/*",
		"node_modules/*", ".openclaw/*", "logs/*",
	}
	result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
	result = append(result, tools.RecentFiles(e.repoRoot))
	
	return result
}

// buildSpecialTool creates workspace-specific or special tools.
func (e *Engine) buildSpecialTool(toolName string) *agent.Tool {
	switch toolName {
	case "repo_map":
		ignorePatterns := []string{
			"*.log", "*.tmp", ".git/*", "vendor/*",
			"node_modules/*", ".openclaw/*", "logs/*",
		}
		tool := tools.RepoMap(e.repoRoot, ignorePatterns)
		return &tool
	case "recent_files":
		tool := tools.RecentFiles(e.repoRoot)
		return &tool
	case "deploy":
		tool := tools.Deploy()
		return &tool
	case "spawn_agent":
		if e.agentRegistry != nil {
			tool := e.agentRegistry.SpawnAgentTool()
			return &tool
		}
	case "task_plan":
		if e.repoRoot != "" {
			tool := tools.TaskPlan(e.repoRoot)
			return &tool
		}
	case "scratchpad":
		if e.repoRoot != "" {
			tool := tools.Scratchpad(e.repoRoot, e.AgentName)
			return &tool
		}
	}
	return nil
}

// findStandardTool looks for a tool in the default registry.
func (e *Engine) findStandardTool(toolName string) *agent.Tool {
	// First check the default registry for registered tools. The root is
	// applied to the whole set in buildTools, not here.
	if tool := tools.GetTool(toolName); tool != nil {
		agentTool := tools.ToAgentTool(tool)
		return &agentTool
	}

	// Fallback to legacy All() for backward compatibility
	for _, t := range tools.All() {
		if t.Name == toolName {
			return &t
		}
	}
	return nil
}

// buildMemoryTools returns memory tools matching configured tool names.
func (e *Engine) buildMemoryTools() []agent.Tool {
	var result []agent.Tool
	for _, toolName := range e.AgentConfig.Tools {
		if strings.HasPrefix(toolName, "memory_") {
			for _, t := range memory.AllMemoryTools(e.MemStore) {
				if t.Name == toolName {
					result = append(result, t)
					break
				}
			}
		}
	}
	return result
}

// needsSpawnTools checks if the tool list includes spawn_agent.
func (e *Engine) needsSpawnTools(tools []string) bool {
	for _, t := range tools {
		if t == "spawn_agent" {
			return true
		}
	}
	return false
}