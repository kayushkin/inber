package server

import (
	"context"
	"strings"
	"testing"
)

// A command that forgets the settings handler does not get a server with no
// GET /settings; it gets a refusal at start.
func TestServeRefusesToStartWithoutASettingsHandler(t *testing.T) {
	g := &Server{config: Config{ListenAddr: "127.0.0.1:0"}}
	err := g.Serve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no settings handler") {
		t.Fatalf("Serve with no settings handler = %v, want a refusal naming it", err)
	}
}
