package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kayushkin/inber/internal/buildinfo"
)

// The health answer carries the running binary's commit stamp, so deploy.sh can
// tell the new build from the one it replaced.
func TestHealthReportsTheRunningBuild(t *testing.T) {
	server := &Server{startedAt: time.Now()}
	recorder := httptest.NewRecorder()
	server.handleHealth(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	var body struct {
		Build *buildinfo.Build `json:"build"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body is not JSON: %v\n%s", err, recorder.Body.String())
	}
	if body.Build == nil {
		t.Fatalf("health body has no build field: %s", recorder.Body.String())
	}
	if *body.Build != buildinfo.Current() {
		t.Fatalf("health build = %+v, want buildinfo.Current() = %+v", *body.Build, buildinfo.Current())
	}
}
