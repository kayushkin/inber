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

// turnTerminatorToolName is the name a tool would have to answer to for
// AddChainAndSidebandFields to withhold the chain field from it: ending a turn
// is the one thing nothing can follow.
//
// No path in this repository registers a tool under this name. tools.All(),
// engine.buildDefaultTools and the DefaultRegistry all omit it, and
// engine.findStandardTool returns nil for it, so an agent config that lists it
// does not get it either. The special case below is therefore dormant, and it
// is written as a constant rather than a literal so that the test which pins
// that fact has one thing to refer to.
//
// This name used to appear in the two descriptions below as an instruction to
// the model, which is the defect TestThenDescriptionsNameOnlyOfferedTools now
// pins: the model was told to chain a tool that is not in the enum and cannot
// be dispatched, and each attempt cost a turn plus a rung on the error-recovery
// context ladder. Whether end_turn should become a real registered tool is a
// model-visible contract change and belongs to todo 87399239.
const turnTerminatorToolName = "end_turn"

// buildThenSchema creates the schema for the "then" field with tool names as enum.
// Must be called with the actual tool names available to the agent.
//
// The descriptions must not name a specific tool. The enum already carries the
// only names that can be dispatched, and a name written into prose here cannot
// be checked against it — that is exactly how the model came to be told, twice,
// to chain a tool nothing answers to. "then" is an optional property (it is
// never added to the schema's Required list), so omitting it is what a model
// with no follow-up should do, and saying so needs no tool name at all.
func buildThenSchema(toolNames []string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Chain another tool to run after this one. Avoids wasting an API turn. Use for: verify after write (edit + build), read related files, or any follow-up you already know you need. Omit this field entirely when no follow-up is needed.",
		"properties": map[string]any{
			"tool": map[string]any{
				"type":        "string",
				"enum":        toolNames,
				"description": "Tool to chain next. Must be one of the listed names. Omit the whole field if no follow-up is needed.",
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

// AddChainAndSidebandFields returns copies of tools whose input schemas carry
// the two things a model may write into a call beyond the tool's own arguments:
// the done/note/split sideband fields, and the "then" chain naming a follow-up
// call. The originals are not modified, and each copy keeps its Run.
//
// It is exported because advertising these fields and honouring them have to be
// the same decision. A caller that builds its own request — the OpenAI-served
// turn does — otherwise sends the plain schemas while the dispatch side is
// ready for the fields, or the reverse, and the model finds out by writing one
// and getting nothing back.
//
// end_turn takes the sideband fields but not the chain: there is nothing to
// follow the end of a turn.
func AddChainAndSidebandFields(tools []Tool) []Tool {
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Name)
	}
	thenSchema := buildThenSchema(toolNames)
	sbSchema := sidebandSchema()

	prepared := make([]Tool, len(tools))
	for i, t := range tools {
		prepared[i] = t
		schema := injectFields(t.InputSchema, sbSchema)
		if t.Name != turnTerminatorToolName {
			schema = injectChainField(schema, thenSchema)
		}
		prepared[i].InputSchema = schema
	}
	return prepared
}

// chainedTool represents a parsed "then" field from tool input.
type chainedTool struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// extractChain parses and removes the "then" field from raw tool input JSON.
// Returns the cleaned input (without "then"), the chained tool if one parsed,
// and the raw bytes of the field.
//
// dropped is the reason the field was taken out of the input and cannot be run,
// empty when there was no chain or when it parsed. It is a return value rather
// than a silent nil because "then" is a call the model asked for: the field is
// deleted from the input either way, so a chain that cannot run has to say so
// or it disappears without a word — which is what used to happen, and what the
// only "then" chain on this host ran into.
func extractChain(rawInput string) (cleanInput string, chain *chainedTool, thenRaw []byte, dropped string) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawInput), &parsed); err != nil {
		return rawInput, nil, nil, ""
	}

	thenValue, hasThen := parsed[chainField]
	if !hasThen {
		return rawInput, nil, nil, ""
	}
	delete(parsed, chainField)

	thenBytes, err := json.Marshal(thenValue)
	if err != nil {
		thenBytes = nil
	}

	// Re-marshal clean input. Failing here leaves "then" in the input the
	// primary tool receives, so the field is neither removed nor run.
	cleanBytes, err := json.Marshal(parsed)
	if err != nil {
		return rawInput, nil, thenBytes, "its tool input could not be rewritten without it"
	}
	cleanInput = string(cleanBytes)

	if thenBytes == nil {
		return cleanInput, nil, nil, "it could not be read back out of the tool input"
	}

	// A model that writes the object as text means the same thing by it. The
	// one "then" chain in 95 sessions of transcripts on this host arrived this
	// way — a JSON string wrapping {"tool":…,"input":…} — and was thrown away.
	// Decoding it changes nothing about what runs: the payload is the model's,
	// only its quoting was.
	if text, isText := thenValue.(string); isText {
		var ct chainedTool
		if err := json.Unmarshal([]byte(text), &ct); err != nil || ct.Tool == "" {
			return cleanInput, nil, thenBytes, "it arrived as text that does not read as {tool, input}"
		}
		return cleanInput, &ct, thenBytes, ""
	}

	var ct chainedTool
	if err := json.Unmarshal(thenBytes, &ct); err != nil {
		return cleanInput, nil, thenBytes, "it is not an object with a tool and an input"
	}
	if ct.Tool == "" {
		return cleanInput, nil, thenBytes, "it names no tool"
	}

	return cleanInput, &ct, thenBytes, ""
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
	// chainNotRunTool and chainNotRunReason describe a chain the block carried
	// and that never ran — malformed, naming a tool that does not exist,
	// refused, or attached to a primary call that itself did not run. They are
	// the record of a call the model asked for and did not get, which is the
	// one outcome that must not be inferable only from the absence of a
	// marker. chainNotRunTool is the chain field's own name when the chain was
	// too malformed to name a tool.
	chainNotRunTool   string
	chainNotRunReason string
	// chainNotRunRefused marks the guard as the reason, so the refusal reaches
	// the model in the same wording it has everywhere else.
	chainNotRunRefused bool
}

// chainNotRunNote is what a chain that did not run says on the tool result it
// rode in on.
func (o toolCallOutcome) chainNotRunNote() string {
	if o.chainNotRunReason == "" {
		return ""
	}
	if o.chainNotRunRefused {
		return chainNote(o.chainNotRunTool, RefuseToolCall(o.chainNotRunTool, o.chainNotRunReason))
	}
	return chainNote(o.chainNotRunTool, "not run: "+o.chainNotRunReason)
}

// RefuseToolCall is the one wording for a tool call the guard would not allow,
// shared by every dispatch site so that a refusal reads the same wherever the
// model runs into one.
func RefuseToolCall(tool, reason string) string {
	return fmt.Sprintf("refused: %s was not run — %s", tool, reason)
}

// chainNote is the one shape for what a "then" chain reports back on the tool
// result it rode in on. The chain that ran writes its marker and its output
// below it; every chain that did not run writes its reason here instead, so
// the two read as the same kind of thing in the transcript.
func chainNote(tool, body string) string {
	return fmt.Sprintf("\n\n--- then(%s) %s ---", tool, body)
}

// ExecuteToolCallWithChainAndSideband runs everything one tool_use block asked
// for: the primary call, the "then" chain it may carry, and the done/note/split
// sideband fields. It returns the text the model gets back and whether the
// primary call failed.
//
// It exists for a caller that runs its own request loop instead of this
// package's — the OpenAI-served turn does — so that path dispatches through the
// same code rather than a copy of it. A copy is the defect this file already
// documents one layer down: two parsers of the same field drift, and the one
// that drifts drops the model's instruction without a word.
//
// There is no read-cache stub here. The cache belongs to the Agent, and a
// caller that has none simply runs the primary tool.
func ExecuteToolCallWithChainAndSideband(ctx context.Context, toolMap map[string]Tool, name string, rawInput string, hooks *Hooks, blockID string, sbCallbacks *SidebandCallbacks, refusal func(tool, input string) string) (output string, isError bool) {
	outcome, isError := executeWithChain(ctx, toolMap, name, rawInput, hooks, blockID, sbCallbacks, nil, refusal)
	return outcome.combined, isError
}

// executeWithChain runs a tool and any chained follow-up, combining results.
// Also processes sideband fields (done, note, split) if callbacks are set.
//
// refusal, when set, is asked about the primary call and the chained call
// alike. Both are calls the model asked for, and gating only the first would
// leave "then" as a way around the gate.
//
// cachedPrimaryOutput, when non-nil, stands in for running the primary tool:
// the read cache has already established that the file is in context and
// answers with a stub instead. It replaces the primary tool's output and
// nothing else. The sideband fields and the chained call arrived on the same
// tool_use block and are separate instructions from the model, so they still
// run — skipping them discarded half of what the model asked for while telling
// it nothing.
func executeWithChain(ctx context.Context, toolMap map[string]Tool, name string, rawInput string, hooks *Hooks, blockID string, sbCallbacks *SidebandCallbacks, cachedPrimaryOutput *string, refusal func(tool, input string) string) (outcome toolCallOutcome, isError bool) {
	// Extract sideband fields first, then chain. The sideband fields go to the
	// gate under their own names, inside processSideband — they are a third
	// thing this block asks for, alongside the primary call and the chain, and
	// each of the three has to be refused on its own account.
	cleanInput, sb := extractSideband(rawInput)
	sbSummary := processSideband(ctx, sb, sbCallbacks, refusal)
	cleanInput, chain, thenRaw, dropped := extractChain(cleanInput)
	outcome.primaryInput = cleanInput

	// Every way out of this function goes past here, including the ones that
	// return before the chain is reached, so what the block asked for besides
	// its primary call is reported on the tool result the model reads — once,
	// in one wording, whether or not that primary call ran.
	//
	// The sideband summary is part of that. Its fields have already been put to
	// the gate and, where allowed, applied by this line: a task really was
	// completed, a note really was saved, and reporting it only on the paths
	// where the primary tool succeeded meant a refused or failed call swallowed
	// the record of work that did happen. A field the gate refused is in the
	// same summary, said in the same place, for the same reason — the model
	// asked for it and has to learn what became of it.
	defer func() {
		if sbSummary != "" {
			outcome.combined = sbSummary + "\n" + outcome.combined
		}
		note := outcome.chainNotRunNote()
		if note == "" {
			return
		}
		outcome.combined += note
		if hooks != nil && hooks.OnToolResult != nil {
			// The note is already in outcome.combined, which is the whole of
			// what the model reads, so this second report exists for the
			// session log and the display — and for one consumer that counts
			// rather than renders. engine's OnToolResult increments
			// Turn.ConsecutiveErrors on every result it is handed as an error,
			// and that counter drives the error-recovery context ladder in
			// engine.contextBudget: one error widens memory recall from 6,000
			// tokens to 20,000, three to 35,000, five to 50,000, each of which
			// also rewrites the cached system-prompt prefix and pays for the
			// whole prompt again.
			//
			// So the note counts as a failure only when it is the block's
			// failure. When executeWithChain returns isError, the primary call
			// is the thing that went wrong and both dispatchers report the
			// block themselves — agent_run.go and engine/turn_openai.go each
			// say so at their call site. Flagging the note as well made one
			// refused call carrying a "then" climb the ladder two rungs, so
			// three ordinary policy refusals recalled memory as if there had
			// been six failures. When the primary call succeeded, nothing else
			// reports a failure for this block and the note is the only record
			// that part of what the model asked for did not happen — then it
			// must count, or a session that fails only its chains never widens
			// its recall at all.
			hooks.OnToolResult(blockID+"-chain", outcome.chainNotRunTool, strings.TrimSpace(note), !isError)
		}
	}()

	if dropped != "" {
		outcome.chainNotRunTool = chainField
		outcome.chainNotRunReason = dropped
		if hooks != nil && hooks.OnToolCall != nil {
			hooks.OnToolCall(blockID+"-chain", chainField, thenRaw)
		}
	}

	// A chain that parsed still does not run unless the primary call does. The
	// three returns below all leave it unrun, so each one records that first.
	notRunWithPrimary := func(reason string) {
		if chain == nil {
			return
		}
		outcome.chainNotRunTool = chain.Tool
		outcome.chainNotRunReason = reason
	}

	// Ask the gate about the primary call. A refused call is refused whether or
	// not the read cache could have answered it: the cached stub is still that
	// tool's output.
	//
	// The sideband fields have their own answer from the same gate and are not
	// revisited here. They are instructions in their own right — an allowed
	// primary carrying a `done` is the case a gate on the primary alone cannot
	// see — so a refusal of this call is not on its own a refusal of them.
	// Whether it should ALSO refuse them, and how Assist mode should treat a
	// rider that can reach a subprocess, is the open half of this finding.
	if refusal != nil {
		if reason := refusal(name, cleanInput); reason != "" {
			outcome.combined = RefuseToolCall(name, reason)
			notRunWithPrimary(fmt.Sprintf("the call it was attached to was refused — %s", reason))
			return outcome, true
		}
	}

	// Run primary tool, unless the read cache already answered for it.
	if cachedPrimaryOutput != nil {
		outcome.primaryOutput = *cachedPrimaryOutput
	} else {
		tool, ok := toolMap[name]
		if !ok {
			outcome.combined = fmt.Sprintf("error: unknown tool %q", name)
			notRunWithPrimary(fmt.Sprintf("the call it was attached to names no tool %q", name))
			return outcome, true
		}
		primaryOutput, err := tool.Run(ctx, cleanInput)
		if err != nil {
			outcome.combined = fmt.Sprintf("error: %s", err)
			notRunWithPrimary(fmt.Sprintf("the call it was attached to failed — %s", err))
			return outcome, true
		}
		outcome.primaryOutput = primaryOutput
	}

	if hooks != nil && hooks.OnToolResult != nil {
		hooks.OnToolResult(blockID, name, outcome.primaryOutput, false)
	}

	// The sideband summary is prepended for every path out of here, above.
	primaryReport := outcome.primaryOutput

	// No chain — return primary result
	if chain == nil {
		outcome.combined = primaryReport
		return outcome, false
	}

	// Execute chained tool
	chainTool, ok := toolMap[chain.Tool]
	if !ok {
		outcome.combined = primaryReport
		outcome.chainNotRunTool = chain.Tool
		outcome.chainNotRunReason = "no tool of that name"
		return outcome, false
	}

	if refusal != nil {
		if reason := refusal(chain.Tool, string(chain.Input)); reason != "" {
			outcome.combined = primaryReport
			outcome.chainNotRunTool = chain.Tool
			outcome.chainNotRunReason = reason
			outcome.chainNotRunRefused = true
			return outcome, false
		}
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
