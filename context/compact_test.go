package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactRepoMap(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	
	// Create a sample Go file
	testFile := filepath.Join(tmpDir, "agent.go")
	testCode := `package agent

import (
	"context"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
)

// Agent represents an AI agent
type Agent struct {
	ID string
	Name string
	client *anthropic.Client
}

// Run executes the agent
func (a *Agent) Run(ctx context.Context, model string, msgs []Message) error {
	return nil
}

func HelperFunc(data []byte) string {
	return string(data)
}

const MaxRetries = 3
var DefaultTimeout = 30

type Config struct {
	APIKey string
	Debug bool
	internalField string
}

type Processor interface {
	Process(ctx context.Context, input string) (string, error)
	Validate(input string) bool
}
`
	
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Parse with compact parser
	compactResult, err := parseGoFileCompact(testFile, "agent.go")
	if err != nil {
		t.Fatalf("parseGoFileCompact failed: %v", err)
	}
	
	// Parse with old parser
	oldResult, err := parseGoFile(testFile, "agent.go")
	if err != nil {
		t.Fatalf("parseGoFile failed: %v", err)
	}
	
	// Compact result should be significantly shorter
	compactLen := len(compactResult)
	oldLen := len(oldResult)
	
	reduction := float64(oldLen-compactLen) / float64(oldLen) * 100
	
	t.Logf("Old format: %d bytes", oldLen)
	t.Logf("Compact format: %d bytes", compactLen)
	t.Logf("Reduction: %.1f%%", reduction)
	
	if reduction < 20 {
		t.Errorf("Expected at least 20%% reduction, got %.1f%%", reduction)
	}
	
	// Verify compact format contains key elements
	if !strings.Contains(compactResult, "pkg agent") {
		t.Error("Missing package declaration")
	}
	if !strings.Contains(compactResult, "github.com/anthropics/anthropic-sdk-go") {
		t.Error("Missing third-party import")
	}
	if !strings.Contains(compactResult, "Agent.Run") {
		t.Error("Missing method signature")
	}
	if !strings.Contains(compactResult, "type Agent struct") {
		t.Error("Missing struct type")
	}
	if !strings.Contains(compactResult, "type Processor interface") {
		t.Error("Missing interface type")
	}
	
	// Verify it DOESN'T contain parameter names (compact mode)
	if strings.Contains(compactResult, "ctx context.Context") {
		t.Error("Compact mode should not include parameter names")
	}
	
	t.Logf("\n=== COMPACT OUTPUT ===\n%s", compactResult)
}

func TestStubChunks(t *testing.T) {
	store := NewStore()
	
	// Create a stub chunk
	stub := Chunk{
		ID:       "recent:agent/agent.go",
		Text:     "agent/agent.go (234 lines, 2h ago)",
		Tags:     []string{"recent", "file:agent/agent.go", "agent.go"},
		Source:   "file",
		IsStub:   true,
		StubPath: "agent/agent.go",
	}
	
	if err := store.Add(stub); err != nil {
		t.Fatal(err)
	}
	
	// Retrieve it
	retrieved, exists := store.Get("recent:agent/agent.go")
	if !exists {
		t.Fatal("Failed to retrieve stub chunk")
	}
	
	if !retrieved.IsStub {
		t.Error("Expected IsStub=true")
	}
	
	if retrieved.StubPath != "agent/agent.go" {
		t.Errorf("Wrong StubPath: %s", retrieved.StubPath)
	}
	
	// Verify stub text is compact
	if len(retrieved.Text) > 100 {
		t.Errorf("Stub text too long: %d bytes", len(retrieved.Text))
	}
	
	t.Logf("Stub text: %s", retrieved.Text)
}

func TestConversationSummarizer(t *testing.T) {
	store := NewStore()
	summarizer := NewConversationSummarizer(store, 5) // Summarize every 5 turns
	
	// Add some conversation chunks
	for i := 0; i < 15; i++ {
		source := "user"
		if i%2 == 1 {
			source = "assistant"
		}
		
		chunk := Chunk{
			ID:     fmt.Sprintf("msg:%d", i),
			Text:   fmt.Sprintf("Message %d content", i),
			Tags:   []string{"conversation"},
			Source: source,
		}
		
		if err := store.Add(chunk); err != nil {
			t.Fatal(err)
		}
		
		summarizer.RecordTurn()
	}
	
	// Should trigger summarization at turn 5, 10, 15
	if !summarizer.ShouldSummarize() {
		t.Error("Expected summarization to be needed")
	}
	
	// Get conversation chunks
	convChunks := summarizer.GetConversationChunks()
	if len(convChunks) != 15 {
		t.Errorf("Expected 15 conversation chunks, got %d", len(convChunks))
	}
	
	// Compact history
	if err := summarizer.CompactConversationHistory(); err != nil {
		t.Fatal(err)
	}
	
	// Should now have fewer chunks + a summary stub
	allChunks := store.ListAll()
	
	// Count conversation chunks (not summary stubs)
	convCount := 0
	stubCount := 0
	for _, chunk := range allChunks {
		for _, tag := range chunk.Tags {
			if tag == "conversation" {
				convCount++
			}
			if tag == "stub" {
				stubCount++
			}
		}
	}
	
	if convCount != 10 {
		t.Errorf("Expected 10 conversation chunks after compaction, got %d", convCount)
	}
	
	if stubCount != 1 {
		t.Errorf("Expected 1 summary stub, got %d", stubCount)
	}
	
	t.Logf("After compaction: %d conversation chunks, %d summary stubs", convCount, stubCount)
}

func BenchmarkRepoMapCompact(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	
	testCode := `package test
import (
	"context"
	"fmt"
)

type Server struct {
	Name string
	Port int
}

func (s *Server) Start(ctx context.Context) error {
	return nil
}

func (s *Server) Stop() error {
	return nil
}
`
	
	os.WriteFile(testFile, []byte(testCode), 0644)
	
	b.Run("Compact", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			parseGoFileCompact(testFile, "test.go")
		}
	})
	
	b.Run("Old", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			parseGoFile(testFile, "test.go")
		}
	})
}
