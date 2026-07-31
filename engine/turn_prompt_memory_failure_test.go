package engine

import (
	"errors"
	"strings"
	"testing"

	memorystore "github.com/kayushkin/memory-store"

	sessionMod "github.com/kayushkin/inber/session"
)

// failingMemoryStore answers the first buildsWithMemories calls with a fixed set
// of memories and every call after that with an error. The real store's only
// error path is its own SELECT, so a transient database failure looks exactly
// like this: a store that worked a moment ago and does not right now.
type failingMemoryStore struct {
	emptyMemoryStore
	memories           []memorystore.Memory
	buildsWithMemories int
	calls              int
}

func (s *failingMemoryStore) BuildContext(memorystore.BuildContextRequest) ([]memorystore.Memory, int, error) {
	s.calls++
	if s.calls > s.buildsWithMemories {
		return nil, 0, errors.New("database is locked")
	}
	return s.memories, 100, nil
}

func identityMemories() []memorystore.Memory {
	return []memorystore.Memory{
		{ID: "identity-1", Content: "You are Claxon. Never touch production.", Importance: 1.0},
	}
}

// A turn whose memory lookup fails must not become a turn with no system prompt.
// The engine used to log a warning and hand the agent nil blocks, so the model
// answered that turn with no identity, no instructions and no tool memories —
// and, because the cached prefix went with them, the user paid full price for
// the input as well.
func TestTurnKeepsItsSystemPromptWhenTheMemoryStoreFails(t *testing.T) {
	store := &failingMemoryStore{memories: identityMemories(), buildsWithMemories: 1}
	e := &Engine{MemStore: store}

	healthy, err := e.buildTurnContext("first question")
	if err != nil {
		t.Fatalf("healthy turn returned an error: %v", err)
	}
	if !blocksContain(healthy, "Never touch production") {
		t.Fatalf("healthy turn did not carry the identity memory: %v", healthy)
	}

	degraded, err := e.buildTurnContext("second question")
	if err != nil {
		t.Fatalf("turn failed outright though the previous context was available: %v", err)
	}
	if !blocksContain(degraded, "Never touch production") {
		t.Fatalf("turn lost its system prompt when the memory store failed: %v", degraded)
	}
}

// Complement to the test above, and the reason the fallback is not simply
// "carry on with nothing": with no previous context to reuse there is no
// degraded turn to run, only a turn that would answer as some other agent. It
// has to fail instead, and say why.
func TestTurnFailsWhenTheMemoryStoreFailsBeforeAnyContextExists(t *testing.T) {
	store := &failingMemoryStore{memories: identityMemories(), buildsWithMemories: 0}
	e := &Engine{MemStore: store}

	blocks, err := e.buildTurnContext("first question")
	if err == nil {
		t.Fatalf("first turn built a prompt from a failed memory store: %v", blocks)
	}
	if len(blocks) != 0 {
		t.Fatalf("failed turn still handed blocks to the caller: %v", blocks)
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("error dropped the store's own reason: %v", err)
	}
}

// Complement: the fallback must be a fallback. A healthy store's second turn
// has to reflect what the store says now, or the fix would pass by serving the
// first turn's context forever.
func TestHealthyTurnsRebuildTheirSystemPrompt(t *testing.T) {
	store := &failingMemoryStore{memories: identityMemories(), buildsWithMemories: 2}
	e := &Engine{MemStore: store}

	if _, err := e.buildTurnContext("first question"); err != nil {
		t.Fatalf("first turn returned an error: %v", err)
	}
	store.memories = []memorystore.Memory{
		{ID: "identity-2", Content: "You are Bran. Deploy on Fridays.", Importance: 1.0},
	}

	second, err := e.buildTurnContext("second question")
	if err != nil {
		t.Fatalf("second turn returned an error: %v", err)
	}
	if !blocksContain(second, "Deploy on Fridays") {
		t.Fatalf("second turn served a stale system prompt: %v", second)
	}
	if blocksContain(second, "Never touch production") {
		t.Fatalf("second turn kept the first turn's memories: %v", second)
	}
}

// The point of reusing the previous turn's blocks rather than rebuilding
// something equivalent: the prefix the model sees has to be byte-identical, or
// the degraded turn also loses the prompt cache it was meant to protect.
func TestReusedSystemPromptHitsTheCachedPrefix(t *testing.T) {
	store := &failingMemoryStore{memories: identityMemories(), buildsWithMemories: 1}
	e := &Engine{MemStore: store}

	healthy, err := e.buildTurnContext("first question")
	if err != nil {
		t.Fatalf("healthy turn returned an error: %v", err)
	}
	e.buildSystemBlocks(healthy)
	cached := e.Cache.LastStablePrefix
	if cached == nil {
		t.Fatal("healthy turn stored no stable prefix to reuse")
	}

	degraded, err := e.buildTurnContext("second question")
	if err != nil {
		t.Fatalf("degraded turn returned an error: %v", err)
	}
	sent := e.buildSystemBlocks(degraded)
	if e.Cache.LastStablePrefix != cached {
		t.Fatal("degraded turn replaced the stable prefix instead of hitting it")
	}
	if len(sent) != len(cached.blocks) {
		t.Fatalf("degraded turn sent %d system blocks, cached prefix holds %d", len(sent), len(cached.blocks))
	}
	for i := range sent {
		if sent[i].Text != cached.blocks[i].Text {
			t.Fatalf("system block %d is not byte-identical to the cached prefix", i)
		}
	}
}

func blocksContain(blocks []sessionMod.NamedBlock, want string) bool {
	for _, b := range blocks {
		if strings.Contains(b.Text, want) {
			return true
		}
	}
	return false
}
