package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestModelsList(t *testing.T) {
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runModelsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "Claude") {
		t.Errorf("expected output to contain 'Claude', got: %s", output)
	}
}