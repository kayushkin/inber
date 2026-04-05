// Package apiutil provides API error detection utilities.
package apiutil

// IsThinkingSignatureError checks if an API error is due to invalid thinking signatures.
// This happens when API credentials rotate mid-session.
func IsThinkingSignatureError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// The Anthropic API returns a generic "Error" message for thinking signature mismatches
	return msg == "Error"
}
