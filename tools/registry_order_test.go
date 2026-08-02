package tools

import (
	"strings"
	"testing"
)

// A tool set goes on the wire as an ordered block, and agent/agent_run.go puts
// the cache_control breakpoint on its last entry. A registry that handed back a
// fresh permutation on every call would therefore move the breakpoint and re-key
// the cached prefix each time it was asked — so the order is contract, not
// presentation, and nothing may rely on being read only once.
func TestRegistryReturnsToolsInTheSameOrderEveryCall(t *testing.T) {
	first := strings.Join(DefaultRegistry.Names(), ",")
	if first == "" {
		t.Fatal("default registry is empty; this test would pass on nothing")
	}
	for i := 0; i < 200; i++ {
		if got := strings.Join(DefaultRegistry.Names(), ","); got != first {
			t.Fatalf("Names() changed on call %d.\nfirst: %s\nnow:   %s", i+2, first, got)
		}
		names := make([]string, 0, DefaultRegistry.Count())
		for _, tl := range DefaultRegistry.List() {
			names = append(names, tl.Name())
		}
		if got := strings.Join(names, ","); got != first {
			t.Fatalf("List() disagreed with Names() on call %d.\nNames: %s\nList:  %s", i+2, first, got)
		}
	}
}
