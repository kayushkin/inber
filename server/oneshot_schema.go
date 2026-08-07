package server

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// buildOneShotParams turns a OneShotRequest into the request that goes to
// Anthropic, and is where every defaulting and translation decision for this
// endpoint lives.
//
// It is a function rather than inline handler code so that what reaches the
// wire can be asserted without making an API call. The handler holds the one
// anthropic.NewClient outside the agent package, so testing this through the
// handler would mean either a network call or an injection seam that exists
// only for tests; splitting the translation out costs neither.
//
// A refused schema is returned as an error and answered with a 400. Failing
// before the call is the point: the alternative is what the endpoint did until
// now, which was to spend a request finding out that the schema it had built
// was invalid.
func buildOneShotParams(req OneShotRequest) (anthropic.MessageNewParams, error) {
	model := req.Model
	if model == "" {
		model = defaultOneShotModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultOneShotMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
		},
	}
	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.SystemPrompt}}
	}

	if len(req.Schema) == 0 {
		return params, nil
	}

	// Schema → synthetic "output" tool with tool_choice forced to it.
	inputSchema, err := toolInputSchema(req.Schema)
	if err != nil {
		return anthropic.MessageNewParams{}, fmt.Errorf("invalid schema: %w", err)
	}
	params.Tools = []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "output",
				Description: anthropic.String("Respond by calling this tool with structured output matching the schema."),
				InputSchema: inputSchema,
			},
		},
	}
	params.ToolChoice = anthropic.ToolChoiceUnionParam{
		OfTool: &anthropic.ToolChoiceToolParam{
			Name: "output",
			Type: "tool",
		},
	}
	return params, nil
}

// toolInputSchema converts a caller's JSON Schema into the SDK's tool
// input-schema parameter without dropping anything the caller sent.
//
// The SDK spells an input schema as three named fields (Type, Properties,
// Required) plus ExtraFields for everything else, while the wire format — and
// msg.OneShotRequest.Schema, the canonical type — is one JSON Schema object.
// Something has to translate between those two shapes, and until this function
// existed nothing did: the handler assigned the caller's whole schema to
// Properties, so a request carrying the documented shape reached Anthropic as
//
//	"input_schema": {"type":"object","properties":{
//	    "type":"object", "properties":{...}, "required":[...]}}
//
// which is three properties named after JSON Schema keywords, with "required"
// dropped. Measured against the live API: that is a hard 400,
// "JSON schema is invalid. It must match JSON Schema draft 2020-12" — so
// schema-forced one-shot, the only reason the endpoint takes a Schema at all,
// could not have worked for any caller honouring the contract.
//
// Keywords this function does not name are carried through in ExtraFields
// rather than discarded, because a translation layer that silently drops
// $defs, additionalProperties or a description is a lossier contract than the
// one the caller was promised. Only the three the SDK models by hand are
// unpacked; the rest travel unchanged.
//
// Every rejection below is a 400 rather than a repair. A schema this function
// cannot represent faithfully is one the caller is wrong about, and guessing
// which of two readings they meant is how the original defect stayed invisible:
// a bare properties map — the undocumented shape — happens to marshal into
// something Anthropic accepts, so accepting both would leave the two contracts
// silently forked again with no way to tell which one a caller believed.
func toolInputSchema(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf("schema must be a JSON Schema object: %w", err)
	}

	out := anthropic.ToolInputSchemaParam{}

	// "type":"object" is required, and requiring it is what separates a JSON
	// Schema from the bare properties map this endpoint used to be fed.
	//
	// Without it the two are not reliably distinguishable: {"properties":{...}}
	// is a legal object schema AND a legal bare map holding one property called
	// "properties". A schema is refused rather than sniffed, because sniffing is
	// how the endpoint came to have two live contracts in the first place. Every
	// schema generator emits the keyword, so the cost of insisting on it is a
	// clear 400 for a caller who is one keyword away from being right.
	rawType, ok := fields["type"]
	if !ok {
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf(`schema has no "type": a tool input schema must be a JSON Schema declaring "type":"object", not a bare map of property definitions`)
	}
	var typeName string
	if err := json.Unmarshal(rawType, &typeName); err != nil {
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf(`schema "type" must be a string: %w`, err)
	}
	if typeName != "object" {
		// The SDK types this field as the constant "object" and emits that
		// whatever the caller asked for, so accepting "array" here would answer
		// a request nobody made.
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf(`schema "type" is %q, but a tool input schema must be an object`, typeName)
	}
	delete(fields, "type")

	if rawProps, ok := fields["properties"]; ok {
		var properties map[string]any
		if err := json.Unmarshal(rawProps, &properties); err != nil {
			return anthropic.ToolInputSchemaParam{}, fmt.Errorf(`schema "properties" must be an object: %w`, err)
		}
		out.Properties = properties
		delete(fields, "properties")
	}

	if rawRequired, ok := fields["required"]; ok {
		var required []string
		if err := json.Unmarshal(rawRequired, &required); err != nil {
			return anthropic.ToolInputSchemaParam{}, fmt.Errorf(`schema "required" must be an array of property names: %w`, err)
		}
		out.Required = required
		delete(fields, "required")
	}

	// Whatever is left is a keyword the SDK has no field for. Carry it.
	if len(fields) > 0 {
		out.ExtraFields = make(map[string]any, len(fields))
		for name, value := range fields {
			var decoded any
			if err := json.Unmarshal(value, &decoded); err != nil {
				return anthropic.ToolInputSchemaParam{}, fmt.Errorf("schema %q is not valid JSON: %w", name, err)
			}
			out.ExtraFields[name] = decoded
		}
	}

	return out, nil
}
