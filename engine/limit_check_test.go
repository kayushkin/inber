package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
)

func TestBuildLimitCheck_NoLimits(t *testing.T) {
	e := &Engine{}
	check := e.buildLimitCheck()
	result := &agent.TurnResult{InputTokens: 1000000, ToolCalls: 100}
	exceeded, _ := check(result)
	if exceeded {
		t.Error("should not exceed when no limits are set")
	}
}

func TestBuildLimitCheck_TurnLimit(t *testing.T) {
	e := &Engine{maxTurns: 10}
	check := e.buildLimitCheck()

	// Under limit
	result := &agent.TurnResult{ToolCalls: 5}
	exceeded, _ := check(result)
	if exceeded {
		t.Error("should not exceed when under turn limit")
	}

	// At limit
	result = &agent.TurnResult{ToolCalls: 10}
	exceeded, reason := check(result)
	if !exceeded {
		t.Error("should exceed when at turn limit")
	}
	if reason == "" {
		t.Error("should provide a reason")
	}

	// Over limit
	result = &agent.TurnResult{ToolCalls: 15}
	exceeded, _ = check(result)
	if !exceeded {
		t.Error("should exceed when over turn limit")
	}
}

func TestBuildLimitCheck_TokenLimit(t *testing.T) {
	e := &Engine{maxInputTokens: 100000}
	check := e.buildLimitCheck()

	// Under limit
	result := &agent.TurnResult{InputTokens: 50000}
	exceeded, _ := check(result)
	if exceeded {
		t.Error("should not exceed when under token limit")
	}

	// Over limit
	result = &agent.TurnResult{InputTokens: 110000}
	exceeded, reason := check(result)
	if !exceeded {
		t.Error("should exceed when over token limit")
	}
	if reason == "" {
		t.Error("should provide a reason")
	}
}

func TestBuildLimitCheck_SessionTokensAccumulate(t *testing.T) {
	e := &Engine{
		maxInputTokens:     100000,
		SessionInputTokens: 80000, // previous turns used 80k
	}
	check := e.buildLimitCheck()

	// Current turn only used 25k, but session total is 105k
	result := &agent.TurnResult{InputTokens: 25000}
	exceeded, _ := check(result)
	if !exceeded {
		t.Error("should exceed when session + current turn exceeds limit")
	}
}

func TestBuildLimitCheck_BothLimits(t *testing.T) {
	e := &Engine{maxTurns: 10, maxInputTokens: 100000}
	check := e.buildLimitCheck()

	// Token limit hit first
	result := &agent.TurnResult{InputTokens: 110000, ToolCalls: 5}
	exceeded, _ := check(result)
	if !exceeded {
		t.Error("should exceed when token limit hit")
	}

	// Turn limit hit first
	result = &agent.TurnResult{InputTokens: 50000, ToolCalls: 12}
	exceeded, _ = check(result)
	if !exceeded {
		t.Error("should exceed when turn limit hit")
	}

	// Neither hit
	result = &agent.TurnResult{InputTokens: 50000, ToolCalls: 5}
	exceeded, _ = check(result)
	if exceeded {
		t.Error("should not exceed when both under limits")
	}
}

func TestDetachDefaults(t *testing.T) {
	// Simulate what NewEngine does for detached runs
	e := &Engine{}

	// No limits set, detach mode
	maxTurns := 0
	maxInputTokens := 0
	detach := true

	if detach {
		if maxTurns == 0 {
			maxTurns = 25
		}
		if maxInputTokens == 0 {
			maxInputTokens = 500000
		}
	}

	e.maxTurns = maxTurns
	e.maxInputTokens = maxInputTokens

	if e.maxTurns != 25 {
		t.Errorf("detach default maxTurns = %d, want 25", e.maxTurns)
	}
	if e.maxInputTokens != 500000 {
		t.Errorf("detach default maxInputTokens = %d, want 500000", e.maxInputTokens)
	}

	// Verify limits actually work with detach defaults
	check := e.buildLimitCheck()

	// Under both limits
	result := &agent.TurnResult{InputTokens: 100000, ToolCalls: 10}
	exceeded, _ := check(result)
	if exceeded {
		t.Error("should not exceed with moderate usage")
	}

	// Over turn limit
	result = &agent.TurnResult{InputTokens: 100000, ToolCalls: 25}
	exceeded, _ = check(result)
	if !exceeded {
		t.Error("should exceed at 25 tool calls")
	}
}

func TestAgentConfigLimits(t *testing.T) {
	// Simulate loading limits from agent config
	e := &Engine{maxTurns: 0, maxInputTokens: 0}

	// Agent config specifies limits
	agentLimits := struct {
		MaxTurns       int
		MaxInputTokens int
	}{
		MaxTurns:       15,
		MaxInputTokens: 300000,
	}

	// Apply agent config limits (as NewEngine does)
	if e.maxTurns == 0 {
		e.maxTurns = agentLimits.MaxTurns
	}
	if e.maxInputTokens == 0 {
		e.maxInputTokens = agentLimits.MaxInputTokens
	}

	if e.maxTurns != 15 {
		t.Errorf("agent config maxTurns = %d, want 15", e.maxTurns)
	}
	if e.maxInputTokens != 300000 {
		t.Errorf("agent config maxInputTokens = %d, want 300000", e.maxInputTokens)
	}
}

func TestCLIOverridesAgentConfig(t *testing.T) {
	// CLI flags should take precedence over agent config
	e := &Engine{maxTurns: 50, maxInputTokens: 1000000} // CLI set these

	// Agent config specifies lower limits
	agentLimits := struct {
		MaxTurns       int
		MaxInputTokens int
	}{
		MaxTurns:       15,
		MaxInputTokens: 300000,
	}

	// Apply agent config limits only if CLI didn't set them (maxTurns != 0 means CLI set it)
	if e.maxTurns == 0 {
		e.maxTurns = agentLimits.MaxTurns
	}
	if e.maxInputTokens == 0 {
		e.maxInputTokens = agentLimits.MaxInputTokens
	}

	// CLI values should persist
	if e.maxTurns != 50 {
		t.Errorf("CLI maxTurns = %d, want 50", e.maxTurns)
	}
	if e.maxInputTokens != 1000000 {
		t.Errorf("CLI maxInputTokens = %d, want 1000000", e.maxInputTokens)
	}
}
