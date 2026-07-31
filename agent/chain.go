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

// toolCallOutcome is everything one tool_use block produced. A block can carry
// two real tool calls — the primary one and the one named in its "then" chain —
// so the two are reported apart and not only as the combined text the model
// gets back. The read cache needs them apart: a chained read's line count is
// not the primary read's, and a chained write invalidates a cached file just as
// a primary one does.
type toolCallOutcome struct {
	// combined is what goes back to the model: the sideband summary, the
	// primary tool's output, and the chained tool's output under its marker.
	combined string
	// primaryInput is the primary tool's own arguments, with the sideband and
	// chain fields removed.
	primaryInput string
	// primaryOutput is the primary tool's own output, with nothing prepended or
	// appended.
	primaryOutput string
	// chainTool names the tool the chain actually ran. It is empty when the
	// block carried no chain, or named a tool that does not exist.
	chainTool string
	// chainInput and chainOutput are that chained call's arguments and result.
	chainInput  string
	chainOutput string
	// chainFailed reports that the chained tool returned an error.
	chainFailed bool
}

// executeWithChain runs a tool and any chained follow-up, combining results.
// Also processes sideband fields (done, note, split) if callbacks are set.
//
// cachedPrimaryOutput, when non-nil, stands in for running the primary tool:
// the read cache has already established that the file is in context and
// answers with a stub instead. It replaces the primary tool's output and
// nothing else. The sideband fields and the chained call arrived on the same
// tool_use block and are separate instructions from the model, so they still
// run — skipping them discarded half of what the model asked for while telling
// it nothing.
func executeWithChain(ctx context.Context, toolMap map[string]Tool, name string, rawInput string, hooks *Hooks, blockID string, sbCallbacks *SidebandCallbacks, cachedPrimaryOutput *string) (outcome toolCallOutcome, isError bool) {
	// Extract sideband fields first, then chain.
	cleanInput, sb := extractSideband(rawInput)
	sbSummary := processSideband(sb, sbCallbacks)
	cleanInput, chain := extractChain(cleanInput)
	outcome.primaryInput = cleanInput

	// Run primary tool, unless the read cache already answered for it.
	if cachedPrimaryOutput != nil {
		outcome.primaryOutput = *cachedPrimaryOutput
	} else {
		tool, ok := toolMap[name]
		if !ok {
			outcome.combined = fmt.Sprintf("error: unknown tool %q", name)
			return outcome, true
		}
		primaryOutput, err := tool.Run(ctx, cleanInput)
		if err != nil {
			outcome.combined = fmt.Sprintf("error: %s", err)
			return outcome, true
		}
		outcome.primaryOutput = primaryOutput
	}

	if hooks != nil && hooks.OnToolResult != nil {
		hooks.OnToolResult(blockID, name, outcome.primaryOutput, false)
	}

	// Prepend sideband summary if any
	primaryReport := outcome.primaryOutput
	if sbSummary != "" {
		primaryReport = sbSummary + "\n" + primaryReport
	}

	// No chain — return primary result
	if chain == nil {
		outcome.combined = primaryReport
		return outcome, false
	}

	// Execute chained tool
	chainTool, ok := toolMap[chain.Tool]
	if !ok {
		outcome.combined = primaryReport + fmt.Sprintf("\n\n--- then(%s) error: unknown tool ---", chain.Tool)
		return outcome, false
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

	outcome.chainTool = chain.Tool
	outcome.chainInput = string(chain.Input)
	outcome.chainOutput = chainOutput
	outcome.chainFailed = chainErr != nil

	// Combine results
	var combined strings.Builder
	combined.WriteString(primaryReport)
	combined.WriteString(fmt.Sprintf("\n\n--- then(%s) ---\n", chain.Tool))
	combined.WriteString(chainOutput)
	outcome.combined = combined.String()

	return outcome, false
}
