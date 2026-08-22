package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// orderedFakeClient returns its tools in the exact order it was given them, so a
// test using it isolates the order the *registry* imposes on its clients from
// the order a client imposes on its own tools. MockClient cannot do that job:
// it stores tools in a map and ranges it, which is the very thing under test.
type orderedFakeClient struct {
	tools    []ToolInfo
	closeErr error
}

var _ MCPClient = (*orderedFakeClient)(nil)

func newOrderedFakeClient(names ...string) *orderedFakeClient {
	c := &orderedFakeClient{}
	for _, n := range names {
		c.tools = append(c.tools, ToolInfo{Name: n, Description: "fake " + n})
	}
	return c
}

func (c *orderedFakeClient) ListTools() []ToolInfo { return c.tools }

func (c *orderedFakeClient) HasTool(name string) bool {
	for _, t := range c.tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (c *orderedFakeClient) CallTool(ctx context.Context, name, input string) (string, error) {
	return "", nil
}

func (c *orderedFakeClient) Close() error { return c.closeErr }

// toolNames flattens a tool slice to its names so an ordering can be compared as
// one string rather than element by element.
func toolNames(tools []ToolInfo) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return names
}

func registryToolNames(r *MCPToolRegistry) []string {
	var out []string
	for _, t := range r.GetAllTools() {
		out = append(out, t.Name)
	}
	return out
}

// --- rig guard -------------------------------------------------------------
//
// Every assertion below is "this order does not change". A fake whose own order
// wandered would make them all fail for the wrong reason, and a fake that
// returned nothing would make them all pass for the wrong reason. Pin both.

func TestTheOrderedFakeIsActuallyOrderedAndNonEmpty(t *testing.T) {
	c := newOrderedFakeClient("gamma", "alpha", "beta")
	want := []string{"gamma", "alpha", "beta"}
	for i := 0; i < 50; i++ {
		got := toolNames(c.ListTools())
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("the fake reordered itself on call %d: got %v want %v", i, got, want)
		}
	}
	if len(c.ListTools()) == 0 {
		t.Fatal("the fake returned no tools, so every ordering assertion here would be vacuous")
	}
}

// --- the defect: the registry ranges a map ---------------------------------

func TestGetAllToolsReturnsTheSameOrderOnEveryCall(t *testing.T) {
	r := NewMCPToolRegistry()
	r.AddClient("delta", newOrderedFakeClient("d1"))
	r.AddClient("alpha", newOrderedFakeClient("a1"))
	r.AddClient("charlie", newOrderedFakeClient("c1"))
	r.AddClient("bravo", newOrderedFakeClient("b1"))
	r.AddClient("echo", newOrderedFakeClient("e1"))

	first := registryToolNames(r)
	for i := 1; i < 200; i++ {
		got := registryToolNames(r)
		if fmt.Sprint(got) != fmt.Sprint(first) {
			t.Fatalf("GetAllTools changed order between calls: call 0 gave %v, call %d gave %v", first, i, got)
		}
	}
}

func TestGetAllToolsOrdersClientsByName(t *testing.T) {
	r := NewMCPToolRegistry()
	r.AddClient("delta", newOrderedFakeClient("d1"))
	r.AddClient("alpha", newOrderedFakeClient("a1"))
	r.AddClient("charlie", newOrderedFakeClient("c1"))

	want := []string{"a1", "c1", "d1"}
	got := registryToolNames(r)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("clients are not walked in name order: got %v want %v", got, want)
	}
}

func TestGetAllToolsKeepsEachClientsOwnToolOrder(t *testing.T) {
	r := NewMCPToolRegistry()
	r.AddClient("alpha", newOrderedFakeClient("a3", "a1", "a2"))
	r.AddClient("bravo", newOrderedFakeClient("b2", "b1"))

	want := []string{"a3", "a1", "a2", "b2", "b1"}
	got := registryToolNames(r)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("a client's own tool order was not preserved: got %v want %v", got, want)
	}
}

// GetTool walks the same map. When two clients offer a tool of the same name,
// which one answers is today a coin flip per call. This pins that the answer is
// the same every time; it deliberately does NOT decide whether a duplicate name
// should be an error at all, which is card e2d0b07b's open question.
func TestGetToolResolvesADuplicateNameToTheSameClientEveryTime(t *testing.T) {
	r := NewMCPToolRegistry()
	for _, name := range []string{"delta", "alpha", "charlie", "bravo", "echo"} {
		c := newOrderedFakeClient("shared")
		c.tools[0].Description = "from " + name
		r.AddClient(name, c)
	}

	first := r.GetTool("shared")
	if first == nil {
		t.Fatal("GetTool found nothing, so this test cannot see a wrong answer")
	}
	for i := 1; i < 200; i++ {
		got := r.GetTool("shared")
		if got == nil {
			t.Fatalf("GetTool returned nil on call %d", i)
		}
		if got.Description != first.Description {
			t.Fatalf("GetTool picked a different client between calls: %q then %q", first.Description, got.Description)
		}
	}
	if first.Description != "from alpha" {
		t.Fatalf("GetTool should resolve to the first client by name, got %q", first.Description)
	}
}

// Close reports one error out of however many failed. Ranging a map makes which
// one nondeterministic, so the same broken state reports a different cause per
// run.
func TestCloseReportsTheSameErrorEveryTime(t *testing.T) {
	build := func() *MCPToolRegistry {
		r := NewMCPToolRegistry()
		for _, name := range []string{"delta", "alpha", "charlie", "bravo", "echo"} {
			c := newOrderedFakeClient("t")
			c.closeErr = errors.New("close failed in " + name)
			r.AddClient(name, c)
		}
		return r
	}

	first := build().Close()
	if first == nil {
		t.Fatal("Close reported success while every client failed")
	}
	for i := 1; i < 200; i++ {
		got := build().Close()
		if got == nil || got.Error() != first.Error() {
			t.Fatalf("Close reported a different error between runs: %v then %v", first, got)
		}
	}
	if first.Error() != "close failed in alpha" {
		t.Fatalf("Close should report the first client by name, got %q", first)
	}
}

// --- the same defect one layer down: Client.ListTools ranges a map ----------

func TestClientListToolsReturnsTheSameOrderOnEveryCall(t *testing.T) {
	c := &Client{tools: map[string]ToolInfo{
		"delta":   {Name: "delta"},
		"alpha":   {Name: "alpha"},
		"charlie": {Name: "charlie"},
		"bravo":   {Name: "bravo"},
		"echo":    {Name: "echo"},
	}}

	first := toolNames(c.ListTools())
	for i := 1; i < 200; i++ {
		got := toolNames(c.ListTools())
		if fmt.Sprint(got) != fmt.Sprint(first) {
			t.Fatalf("Client.ListTools changed order between calls: call 0 gave %v, call %d gave %v", first, i, got)
		}
	}
	want := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	if fmt.Sprint(first) != fmt.Sprint(want) {
		t.Fatalf("Client.ListTools should return tools in name order: got %v want %v", first, want)
	}
}
