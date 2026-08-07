package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// marshalInputSchema renders what actually goes on the wire.
//
// These tests assert the marshalled bytes rather than the struct fields on
// purpose. The defect they cover was invisible at struct level — assigning a
// whole schema to Properties type-checks, reads plausibly, and only becomes
// wrong once the SDK renders it — so a test reading back out.Properties would
// have agreed with the bug.
func marshalInputSchema(t *testing.T, schema anthropic.ToolInputSchemaParam) map[string]any {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}
	return out
}

// wireSchemaFor builds the real request for a caller's schema and returns the
// input_schema the "output" tool carries.
//
// It goes through buildOneShotParams rather than calling toolInputSchema
// directly, because the two layers can fail independently: the conversion can
// be correct while the caller of it builds its schema some other way, which is
// exactly the shape of the defect these tests cover.
func wireSchemaFor(t *testing.T, callerSchema json.RawMessage) map[string]any {
	t.Helper()
	params, err := buildOneShotParams(OneShotRequest{Prompt: "hi", Schema: callerSchema})
	if err != nil {
		t.Fatalf("buildOneShotParams: %v", err)
	}
	if len(params.Tools) != 1 || params.Tools[0].OfTool == nil {
		t.Fatalf("want exactly one custom tool on the request, got %d", len(params.Tools))
	}
	if params.ToolChoice.OfTool == nil || params.ToolChoice.OfTool.Name != "output" {
		t.Errorf("tool_choice does not force the output tool, so the schema is a suggestion rather than a contract")
	}
	return marshalInputSchema(t, params.Tools[0].OfTool.InputSchema)
}

// TestSchemaReachesTheWireAsTheCallerWroteIt is the regression test for the
// defect: a caller sending what msg.OneShotRequest documents — a whole JSON
// Schema — must reach Anthropic as that schema, not nested one level down
// inside "properties".
//
// The shape asserted here was verified against the live API: this one is
// accepted and answered with a conforming tool_use; the pre-fix one is a 400,
// "JSON schema is invalid. It must match JSON Schema draft 2020-12".
func TestSchemaReachesTheWireAsTheCallerWroteIt(t *testing.T) {
	callerSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"summary": {"type": "string", "description": "one line"},
			"score":   {"type": "integer"}
		},
		"required": ["summary", "score"]
	}`)

	wire := wireSchemaFor(t, callerSchema)

	if wire["type"] != "object" {
		t.Errorf(`wire "type" = %v, want "object"`, wire["type"])
	}

	properties, ok := wire["properties"].(map[string]any)
	if !ok {
		t.Fatalf(`wire "properties" is %T, want an object`, wire["properties"])
	}
	for _, name := range []string{"summary", "score"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("the caller's property %q is not on the wire; properties = %v", name, properties)
		}
	}

	// The exact signature of the old defect: the caller's schema landed inside
	// properties, so the JSON Schema keywords became property names. Naming all
	// three keeps the failure legible if it ever comes back by another route.
	for _, keyword := range []string{"type", "properties", "required"} {
		if _, ok := properties[keyword]; ok {
			t.Errorf("the JSON Schema keyword %q is on the wire as a property name — the caller's schema was nested inside properties instead of being used as the schema", keyword)
		}
	}

	required, ok := wire["required"].([]any)
	if !ok {
		t.Fatalf(`wire "required" is %T, want an array — a schema-forced call whose required list is dropped lets the model omit every field the caller asked for`, wire["required"])
	}
	if len(required) != 2 || required[0] != "summary" || required[1] != "score" {
		t.Errorf(`wire "required" = %v, want ["summary" "score"]`, required)
	}
}

// TestKeywordsTheSDKDoesNotModelSurviveTheConversion pins the pass-through.
// The SDK names three fields; a JSON Schema has many more, and a translation
// layer that drops the rest is lossy in a way the caller cannot see.
func TestKeywordsTheSDKDoesNotModelSurviveTheConversion(t *testing.T) {
	callerSchema := json.RawMessage(`{
		"type": "object",
		"description": "the output contract",
		"additionalProperties": false,
		"properties": {"item": {"$ref": "#/$defs/item"}},
		"$defs": {"item": {"type": "string"}}
	}`)

	wire := wireSchemaFor(t, callerSchema)

	if wire["description"] != "the output contract" {
		t.Errorf(`"description" = %v, want it carried through unchanged`, wire["description"])
	}
	if wire["additionalProperties"] != false {
		t.Errorf(`"additionalProperties" = %v, want false carried through`, wire["additionalProperties"])
	}
	defs, ok := wire["$defs"].(map[string]any)
	if !ok {
		t.Fatalf(`"$defs" is %T, want it carried through — a $ref with no $defs is an unresolvable schema`, wire["$defs"])
	}
	if _, ok := defs["item"]; !ok {
		t.Errorf(`"$defs" reached the wire without "item": %v`, defs)
	}
}

// TestASchemaWithNoPropertiesIsNotRejected — a schema that constrains nothing
// is legal, and the caller may be using the tool purely to force a tool_use.
func TestASchemaWithNoPropertiesIsNotRejected(t *testing.T) {
	if wire := wireSchemaFor(t, json.RawMessage(`{"type":"object"}`)); wire["type"] != "object" {
		t.Errorf(`wire "type" = %v, want "object"`, wire["type"])
	}
}

// TestSchemasThisCannotRepresentAreRefused covers the fail-loud half.
//
// Each case is a schema that has no faithful rendering in the SDK's parameter
// type. Answering a 200 to any of them would send the model a contract the
// caller did not write — which is the class of failure this whole change is
// about, so repairing them quietly would reintroduce it one layer up.
func TestSchemasThisCannotRepresentAreRefused(t *testing.T) {
	cases := map[string]string{
		"a bare properties map, the undocumented shape": `{"summary":{"type":"string"}}`,
		"a non-object type":                             `{"type":"array","items":{"type":"string"}}`,
		"type is not a string":                          `{"type":["object","null"]}`,
		"properties is not an object":                   `{"type":"object","properties":["summary"]}`,
		"required is not an array of names":             `{"type":"object","required":"summary"}`,
		"not an object at all":                          `"summary"`,
		"not JSON":                                      `{`,
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := buildOneShotParams(OneShotRequest{Prompt: "hi", Schema: json.RawMessage(raw)}); err == nil {
				t.Errorf("buildOneShotParams with schema %s returned no error, but this schema cannot be represented faithfully — the caller would get a tool contract they did not write", raw)
			}
		})
	}
}

// TestTheHandlerBuildsItsRequestThroughTheSchemaConversion is the control on
// every other test in this file.
//
// The rest exercise buildOneShotParams and toolInputSchema directly, so all of
// them stay green if somebody rewrites the handler to build its own params and
// never call either — a sabotage that removes the *use* of the fixed code
// rather than the code. This drives the real HTTP handler and asserts the one
// thing observable without an API call: a schema the conversion refuses is
// answered 400, before any request is made. If the handler stops routing
// through buildOneShotParams, that 400 becomes a live call with a mangled
// schema and this test says so.
func TestTheHandlerBuildsItsRequestThroughTheSchemaConversion(t *testing.T) {
	body := `{"prompt":"hi","schema":{"summary":{"type":"string"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/oneshot", strings.NewReader(body))
	rec := httptest.NewRecorder()

	(&Server{}).handleOneShot(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleOneShot with an unrepresentable schema = %d, want 400 — the handler is not routing through buildOneShotParams, so a schema the conversion refuses reaches Anthropic instead", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "schema") {
		t.Errorf("the 400 does not mention the schema, so a caller cannot tell what to fix: %s", rec.Body.String())
	}
}

// TestABarePropertiesMapIsRefusedRatherThanAccepted is split out of the table
// above because it is the one case that is NOT obviously broken, and the
// reasoning matters more than the assertion.
//
// {"summary":{"type":"string"}} assigned straight to Properties happens to
// marshal into a schema Anthropic accepts. So the undocumented reading works,
// the documented one 400s, and both look like "the endpoint" to a caller. That
// is precisely why this returns an error instead of a best guess: with two
// readings silently both live, nothing can tell which one a caller believed,
// and the contracts stay forked. inber-server has run no sessions since
// 2026-05-10 and no caller in the fleet sends this shape, so refusing it now
// costs nothing and closes the fork.
func TestABarePropertiesMapIsRefusedRatherThanAccepted(t *testing.T) {
	_, err := buildOneShotParams(OneShotRequest{Prompt: "hi", Schema: json.RawMessage(`{"summary":{"type":"string"}}`)})
	if err == nil {
		t.Fatal("a bare properties map was accepted; the two readings of Schema are still both live")
	}
}
