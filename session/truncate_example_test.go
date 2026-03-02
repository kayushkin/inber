package session_test

import (
	"fmt"
	"strings"

	"github.com/kayushkin/inber/session"
)

// Example showing automatic truncation of large tool results
func ExampleSession_LogToolResult_truncation() {
	// Create a session
	sess, _ := session.New("./logs", "claude-3-5-sonnet-20241022", "example", "")
	defer sess.Close()

	// Configure aggressive truncation for demo
	sess.SetTruncateConfig(session.TruncateConfig{
		Threshold:  100,  // truncate if > 100 tokens (~400 chars)
		HeadTokens: 50,   // show first 50 tokens
		TailTokens: 20,   // show last 20 tokens
		Strategy:   session.StrategyHeadTail,
	})

	// Simulate the Go build error from your example
	largeError := strings.Repeat(
		"router_X.go:123: cannot use val (type *CustomStructX) as type interface{MethodX()}\n"+
			"        *CustomStructX does not implement interface{MethodX()}\n"+
			"                have MethodX(context.Context, *RequestX) (*ResponseX, error)\n"+
			"                want MethodX(context.Context, *RequestX) (ResponseX, error)\n\n",
		20, // 20 identical errors
	)

	fmt.Printf("Original size: %d chars\n", len(largeError))

	// Log it - truncation happens automatically
	sess.LogToolResult("tool-123", "shell", largeError, true)

	// Can still retrieve full output if needed
	full := sess.GetFullToolResult("tool-123")
	fmt.Printf("Full output available: %d chars\n", len(full))
	fmt.Printf("Matches original: %t\n", full == largeError)

	// Output:
	// Original size: 6000 chars
	// Full output available: 6000 chars
	// Matches original: true
}

// Example showing different truncation strategies by agent role
func ExampleTruncateConfigForRole() {
	// Main agent: aggressive truncation
	mainCfg := session.TruncateConfigForRole("main")
	fmt.Printf("Main agent threshold: %d tokens\n", mainCfg.Threshold)

	// Project agent: moderate truncation
	projectCfg := session.TruncateConfigForRole("project")
	fmt.Printf("Project agent threshold: %d tokens\n", projectCfg.Threshold)

	// Run agent: minimal truncation (preserve test output)
	runCfg := session.TruncateConfigForRole("run")
	fmt.Printf("Run agent threshold: %d tokens\n", runCfg.Threshold)

	// Output:
	// Main agent threshold: 1000 tokens
	// Project agent threshold: 3000 tokens
	// Run agent threshold: 5000 tokens
}
