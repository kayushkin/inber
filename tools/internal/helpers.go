// Package internal provides shared helpers for tool subpackages.
package internal

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// Parse unmarshals tool input JSON into the given type.
func Parse[T any](input string) (T, error) {
	var v T
	err := json.Unmarshal([]byte(input), &v)
	return v, err
}

// Props builds an InputSchema from a map of property definitions.
func Props(required []string, properties map[string]any) anthropic.ToolInputSchemaParam {
	s := anthropic.ToolInputSchemaParam{
		Properties: properties,
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

// Str creates a string property definition.
func Str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// Integer creates an integer property definition.
func Integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
