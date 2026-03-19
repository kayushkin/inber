package main

import (
	"os"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
)

func TestBuildSystemPrompt(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	eng := &engine.Engine{}
	// Use exported method to set identity override for raw/override mode testing
	eng.IdentityOverride = "You are a test agent."
	blocks := eng.BuildSystemPrompt("hello")
	found := false
	for _, b := range blocks {
		if strings.Contains(b.Text, "test agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected identity in system prompt blocks")
	}
}