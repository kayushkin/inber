package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// chainField is the JSON field name used for tool chaining.
const chainField = "then"

// buildThenSchema creates the schema for the "then" field with tool names as enum.
// Must be called with the actual tool names available to the agent.
func buildThenSchema(toolNames []string) map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Chain another tool to run after this one. Avoids wasting an API turn. Use for: verify after write (edit + build), read related files, or any follow-up you already know you need. Use end_turn when no follow-up is needed.",
		"properties": map[string]any{
			"tool": map[string]any{
				"type":        "string",
				"enum":        toolNames,
				"description": "Tool to chain next. Use end_turn if no follow-up needed.",
			},
			"input": map[string]any{
				"type":        "object",
				"description": "Input for the chained tool",
			},
		},
		"required": []string{"tool", "input"},
	}
}

// injectFields adds multiple properties to a tool's InputSchema.
func injectFields(schema anthropic.ToolInputSchemaParam, fields map[string]any) anthropic.ToolInputSchemaParam {
	props, ok := schema.Properties.(map[string]any)
	if !ok {
		return schema
	}
	newProps := make(map[string]any, len(props)+len(fields))
	for k, v := range props {
		newProps[k] = v
	}
	for k, v := range fields {
		newProps[k] = v
	}
	return anthropic.ToolInputSchemaParam{
		Properties: newProps,
		Required:   schema.Required,
	}
}

// injectChainField adds the "then" property to a tool's InputSchema.
// Returns a new schema with the field added (does not mutate original).
func injectChainField(schema anthropic.ToolInputSchemaParam, thenSchema map[string]any) anthropic.ToolInputSchemaParam {
	// Clone properties map
	props, ok := schema.Properties.(map[string]any)
	if !ok {
		return schema
	}
	newProps := make(map[string]any, len(props)+1)
	for k, v := range props {
		newProps[k] = v
	}
	newProps[chainField] = thenSchema

	return anthropic.ToolInputSchemaParam{
		Properties: newProps,
		Required:   schema.Required,
		// Type is auto-set to "object" by the SDK
	}
}

// chainedTool represents a parsed "then" field from tool input.
type chainedTool struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// extractChain parses and removes the "then" field from raw tool input JSON.
// Returns the cleaned input (without "then") and the chained tool if present.
func extractChain(rawInput string) (cleanInput string, chain *chainedTool) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawInput), &parsed); err != nil {
		return rawInput, nil
	}

	thenRaw, hasThen := parsed[chainField]
	if !hasThen {
		return rawInput, nil
	}
	delete(parsed, chainField)

	// Re-marshal clean input
	cleanBytes, err := json.Marshal(parsed)
	if err != nil {
		return rawInput, nil
	}

	// Parse the chain
	thenBytes, err := json.Marshal(thenRaw)
	if err != nil {
		return string(cleanBytes), nil
	}
	var ct chainedTool
	if err := json.Unmarshal(thenBytes, &ct); err != nil || ct.Tool == "" {
		return string(cleanBytes), nil
	}

	return string(cleanBytes), &ct
}

// executeWithChain runs a tool and any chained follow-up, combining results.
// Also processes sideband fields (done, note, split) if callbacks are set.
func executeWithChain(ctx context.Context, toolMap map[string]Tool, name string, rawInput string, hooks *Hooks, blockID string, sbCallbacks *SidebandCallbacks) (output string, isError bool) {
	// Extract sideband fields first, then chain.
	cleanInput, sb := extractSideband(rawInput)
	sbSummary := processSideband(sb, sbCallbacks)
	cleanInput, chain := extractChain(cleanInput)

	// Run primary tool
	tool, ok := toolMap[name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", name), true
	}
	primaryOutput, err := tool.Run(ctx, cleanInput)
	if err != nil {
		return fmt.Sprintf("error: %s", err), true
	}

	if hooks != nil && hooks.OnToolResult != nil {
		hooks.OnToolResult(blockID, name, primaryOutput, false)
	}

	// Prepend sideband summary if any
	if sbSummary != "" {
		primaryOutput = sbSummary + "\n" + primaryOutput
	}

	// No chain — return primary result
	if chain == nil {
		return primaryOutput, false
	}

	// Execute chained tool
	chainTool, ok := toolMap[chain.Tool]
	if !ok {
		return primaryOutput + fmt.Sprintf("\n\n--- then(%s) error: unknown tool ---", chain.Tool), false
	}

	if hooks != nil && hooks.OnToolCall != nil {
		hooks.OnToolCall(blockID+"-chain", chain.Tool, chain.Input)
	}

	chainOutput, chainErr := chainTool.Run(ctx, string(chain.Input))
	if chainErr != nil {
		chainOutput = fmt.Sprintf("error: %s", chainErr)
	}

	if hooks != nil && hooks.OnToolResult != nil {
		hooks.OnToolResult(blockID+"-chain", chain.Tool, chainOutput, chainErr != nil)
	}

	// Combine results
	var combined strings.Builder
	combined.WriteString(primaryOutput)
	combined.WriteString(fmt.Sprintf("\n\n--- then(%s) ---\n", chain.Tool))
	combined.WriteString(chainOutput)

	return combined.String(), false
}
