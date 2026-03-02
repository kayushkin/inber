# Automatic Tool Result Truncation

**Quick Reference**

## What It Does

Automatically truncates large tool results (>1000 tokens) to prevent context overflow while preserving full output in session database.

## Example

Your Go build error (80 identical errors, 11K tokens) → Truncated to 700 tokens:

```
router_0.go:10: cannot use val0...
router_1.go:13: cannot use val1...
router_2.go:16: cannot use val2...
... (truncated 10,500 tokens - 74 similar errors) ...
router_77.go:241: cannot use val77...
router_78.go:244: cannot use val78...
router_79.go:247: cannot use val79...
```

**Savings**: 10,300 tokens (36% reduction), full output still in DB.

## Default Thresholds

| Agent Type | Threshold | Strategy |
|------------|-----------|----------|
| `main` | 1000 tokens | Aggressive truncation |
| `project-*` | 3000 tokens | Moderate truncation |
| `run-*` | 5000 tokens | Preserve test output |

Applied automatically based on agent name.

## Configuration

```go
// Custom config
sess.SetTruncateConfig(session.TruncateConfig{
    Threshold:  2000,
    HeadTokens: 1000,
    TailTokens: 500,
    Strategy:   session.StrategyHeadTail,
})

// Disable
sess.SetTruncateConfig(session.TruncateConfig{Threshold: 0})
```

## Retrieving Full Output

```go
full := sess.GetFullToolResult("tool-123")
```

## See Also

- [Full Documentation](../docs/smart-truncation.md)
- [Design Doc](../docs/smart-truncation-design.md)
