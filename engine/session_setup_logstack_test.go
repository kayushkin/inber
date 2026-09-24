package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The logstack URL the engine is configured with reaches the session logger:
// the session's first entry is posted there. The engine used to read
// LOGSTACK_URL from the environment itself.
func TestSetupSessionSendsTheSessionLogToTheLogstackItIsGiven(t *testing.T) {
	posted := make(chan struct{}, 8)
	logstack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted <- struct{}{}
		w.WriteHeader(http.StatusCreated)
	}))
	defer logstack.Close()

	session, _, _, _, _, err := setupSession(t.TempDir(), "testagent", "chat", true, true, logstack.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	select {
	case <-posted:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing reached the logstack setupSession was given")
	}
}
