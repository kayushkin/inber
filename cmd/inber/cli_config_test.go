package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShow(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfigShow(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "Configuration") {
		t.Errorf("expected 'Configuration' in output, got: %s", output)
	}
	if !strings.Contains(output, "Repo root:") {
		t.Errorf("expected 'Repo root:' in output, got: %s", output)
	}
}

func TestConfigInit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfigInit(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	// Verify files were created
	if _, err := os.Stat(filepath.Join(dir, ".inber")); os.IsNotExist(err) {
		t.Error("expected .inber directory to be created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); os.IsNotExist(err) {
		t.Error("expected .env to be created")
	}
}