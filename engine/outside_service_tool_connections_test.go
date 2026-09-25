package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// An agent config that lists "browser" gets a browser that reaches the PinchTab
// the engine was configured with. Before 2026-09-25 the tools package read
// PINCHTAB_URL and PINCHTAB_TOKEN itself; now findStandardTool builds the tool
// from the engine's connections, and dropping them would send it to
// tool-store's default address with no credential, which this test catches.
func TestAConfiguredBrowserReachesTheEnginesPinchtab(t *testing.T) {
	var authorization string
	pinchtab := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.Write([]byte(`[]`))
	}))
	defer pinchtab.Close()

	engine := &Engine{toolConnections: toolstoretools.OutsideServiceConnections{
		Pinchtab: toolstoretools.PinchtabConnection{BaseURL: pinchtab.URL, Token: "pinchtab-token"},
	}}
	browser := engine.findStandardTool("browser")
	if browser == nil {
		t.Fatal("findStandardTool does not resolve browser")
	}
	if _, err := browser.Run(context.Background(), `{"action":"tabs"}`); err != nil {
		t.Fatalf("browser tabs: %v", err)
	}
	if authorization != "Bearer pinchtab-token" {
		t.Fatalf("PinchTab saw Authorization %q, want the engine's token", authorization)
	}
}
