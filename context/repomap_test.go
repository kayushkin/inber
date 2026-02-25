package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRepoMap(t *testing.T) {
	// Create a temporary directory with sample Go files
	tmpDir := t.TempDir()
	
	// Create a sample Go file
	sampleGo := `package main

import "fmt"

type User struct {
	Name string
	Age  int
}

func (u *User) Greet() string {
	return fmt.Sprintf("Hello, %s", u.Name)
}

func main() {
	fmt.Println("Hello, World!")
}
`
	
	err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(sampleGo), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	// Create a non-Go file
	readme := "# Test Project\n\nThis is a test."
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(readme), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	// Build repo map
	repoMap, err := BuildRepoMap(tmpDir, nil)
	if err != nil {
		t.Fatalf("BuildRepoMap failed: %v", err)
	}
	
	// Verify it contains expected elements
	expected := []string{
		"package main",
		"type User struct",
		"func (u *User) Greet()",
		"func main()",
		"README.md",
	}
	
	for _, exp := range expected {
		if !strings.Contains(repoMap, exp) {
			t.Errorf("repo map missing expected content: %s", exp)
		}
	}
}

func TestParseGoFileStructs(t *testing.T) {
	tmpDir := t.TempDir()
	
	goFile := `package test

type Config struct {
	Host string
	Port int
	TLS  bool
}

type Handler interface {
	Handle(req string) error
}
`
	
	filePath := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(filePath, []byte(goFile), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	summary, err := parseGoFile(filePath, "test.go")
	if err != nil {
		t.Fatalf("parseGoFile failed: %v", err)
	}
	
	expected := []string{
		"package test",
		"type Config struct",
		"Host string",
		"Port int",
		"TLS bool",
		"type Handler interface",
		"Handle",
	}
	
	for _, exp := range expected {
		if !strings.Contains(summary, exp) {
			t.Errorf("summary missing: %s\nGot:\n%s", exp, summary)
		}
	}
}

func TestParseGoFileFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	
	goFile := `package test

func Add(a, b int) int {
	return a + b
}

func ProcessData(input []string, callback func(string) error) ([]string, error) {
	return nil, nil
}
`
	
	filePath := filepath.Join(tmpDir, "test.go")
	err := os.WriteFile(filePath, []byte(goFile), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	summary, err := parseGoFile(filePath, "test.go")
	if err != nil {
		t.Fatalf("parseGoFile failed: %v", err)
	}
	
	if !strings.Contains(summary, "func Add(a int, b int) int") {
		t.Errorf("missing Add function signature\nGot:\n%s", summary)
	}
	
	if !strings.Contains(summary, "func ProcessData") {
		t.Errorf("missing ProcessData function\nGot:\n%s", summary)
	}
}
