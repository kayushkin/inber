package conversation

import (
	"encoding/json"
	"fmt"
)

// ToolInputText renders a tool call's arguments as readable JSON text.
//
// anthropic.ToolUseBlockParam.Input is an `any`, and what it actually holds
// depends on where the block came from: a call that arrived from the API is a
// json.RawMessage (ToolUseBlock.ToParam copies the raw bytes straight across),
// while the pruning paths in this package store a plain map. A json.RawMessage
// is a []byte, so fmt's %v renders it as decimal byte codes — "[123 34 97 ...]"
// — which is unreadable to both the model and a human. Every caller that wants
// the arguments as text has to marshal instead, so it lives here once.
func ToolInputText(input any) string {
	if input == nil {
		return ""
	}
	// Already the bytes the model sent. Return them verbatim rather than
	// re-marshalling, so a partial or malformed payload still shows what it
	// held instead of collapsing to an error.
	if raw, ok := input.(json.RawMessage); ok {
		return string(raw)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprintf("[unrenderable tool input: %v]", err)
	}
	return string(data)
}
