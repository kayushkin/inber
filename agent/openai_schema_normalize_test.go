package agent

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// hasKey reports whether the map (walked recursively through maps and slices)
// contains any object with the given key.
func hasKeyDeep(v interface{}, key string) bool {
	switch n := v.(type) {
	case map[string]interface{}:
		if _, ok := n[key]; ok {
			return true
		}
		for _, child := range n {
			if hasKeyDeep(child, key) {
				return true
			}
		}
	case []interface{}:
		for _, child := range n {
			if hasKeyDeep(child, key) {
				return true
			}
		}
	}
	return false
}

func TestNormalizeSchemaForOpenAI_TopLevelOneOf(t *testing.T) {
	schema := map[string]interface{}{
		"oneOf": []interface{}{
			map[string]interface{}{"const": "a"},
			map[string]interface{}{"const": "b"},
		},
	}
	normalizeSchemaForOpenAI(schema)

	if _, ok := schema["oneOf"]; ok {
		t.Fatalf("oneOf should have been removed, got: %v", schema)
	}
	anyOf, ok := schema["anyOf"].([]interface{})
	if !ok {
		t.Fatalf("anyOf should be present as a slice, got: %v", schema["anyOf"])
	}
	if len(anyOf) != 2 {
		t.Fatalf("anyOf should carry both branches, got %d", len(anyOf))
	}
}

func TestNormalizeSchemaForOpenAI_Nested(t *testing.T) {
	// oneOf buried under properties, $defs, and items — all must be rewritten.
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mode": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"const": "fast"},
					map[string]interface{}{"const": "slow"},
				},
			},
			"items": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"oneOf": []interface{}{
						map[string]interface{}{"type": "string"},
						map[string]interface{}{"type": "number"},
					},
				},
			},
		},
		"$defs": map[string]interface{}{
			"variant": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"const": "x"},
				},
			},
		},
	}
	normalizeSchemaForOpenAI(schema)

	if hasKeyDeep(schema, "oneOf") {
		t.Fatalf("no oneOf key should survive anywhere in the schema: %v", schema)
	}
	if !hasKeyDeep(schema, "anyOf") {
		t.Fatalf("expected anyOf keys after rewrite: %v", schema)
	}
}

func TestNormalizeSchemaForOpenAI_BothPresentMerges(t *testing.T) {
	schema := map[string]interface{}{
		"anyOf": []interface{}{
			map[string]interface{}{"type": "string"},
		},
		"oneOf": []interface{}{
			map[string]interface{}{"const": "a"},
			map[string]interface{}{"const": "b"},
		},
	}
	normalizeSchemaForOpenAI(schema)

	if _, ok := schema["oneOf"]; ok {
		t.Fatalf("oneOf should be gone after merge: %v", schema)
	}
	anyOf, ok := schema["anyOf"].([]interface{})
	if !ok {
		t.Fatalf("anyOf should be a slice, got %v", schema["anyOf"])
	}
	if len(anyOf) != 3 {
		t.Fatalf("merge should keep all 1+2 branches, got %d", len(anyOf))
	}
}

func TestNormalizeSchemaForOpenAI_NoOneOfUntouched(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"name"},
	}
	before, _ := json.Marshal(schema)
	normalizeSchemaForOpenAI(schema)
	after, _ := json.Marshal(schema)
	if string(before) != string(after) {
		t.Fatalf("schema without oneOf should be unchanged:\n before=%s\n after =%s", before, after)
	}
}

// TestConvertAnthropicToolsToOpenAI_NormalizesOneOf exercises the full conversion
// path and confirms the emitted OpenAI Parameters carry no oneOf, while the source
// tool's canonical InputSchema is left untouched.
func TestConvertAnthropicToolsToOpenAI_NormalizesOneOf(t *testing.T) {
	inputSchema := anthropic.ToolInputSchemaParam{
		Properties: map[string]interface{}{
			"choice": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"const": "yes"},
					map[string]interface{}{"const": "no"},
				},
			},
		},
	}
	tools := []Tool{{Name: "decide", Description: "d", InputSchema: inputSchema}}

	out := ConvertAnthropicToolsToOpenAI(tools)
	if len(out) != 1 {
		t.Fatalf("expected 1 converted tool, got %d", len(out))
	}
	if hasKeyDeep(out[0].Function.Parameters, "oneOf") {
		t.Fatalf("converted OpenAI parameters must not contain oneOf: %v", out[0].Function.Parameters)
	}
	if !hasKeyDeep(out[0].Function.Parameters, "anyOf") {
		t.Fatalf("converted OpenAI parameters should contain anyOf: %v", out[0].Function.Parameters)
	}

	// The canonical InputSchema must be untouched (no mutation of the source).
	if hasKeyDeep(map[string]interface{}(tools[0].InputSchema.Properties.(map[string]interface{})), "anyOf") {
		t.Fatalf("source InputSchema should not have been mutated to anyOf")
	}
	if !hasKeyDeep(map[string]interface{}(tools[0].InputSchema.Properties.(map[string]interface{})), "oneOf") {
		t.Fatalf("source InputSchema should still carry its original oneOf")
	}
}
