package agent

import (
	"fmt"
	"strings"
)

// sanitizeToolID ensures a tool ID matches Anthropic's pattern ^[a-zA-Z0-9_-]+$
// OpenAI/GLM may generate IDs with dots, colons, or other characters.
func sanitizeToolID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "tool_" + fmt.Sprintf("%d", len(id))
	}
	return result
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}