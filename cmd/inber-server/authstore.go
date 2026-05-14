package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kayushkin/inber/logger"
)

// resolveAnthropicFromAuthStore fetches the Anthropic credential routed to
// the given app from the auth-store service and sets ANTHROPIC_API_KEY in
// the process environment. It must be called before any code reads the env
// var.
//
// Env:
//
//	AUTH_STORE_URL    base URL (default http://127.0.0.1:8303)
//	AUTH_STORE_TOKEN  bearer token (required)
//
// Fails loud on any error — never falls back to ambient ANTHROPIC_API_KEY
// (CLAUDE.md "single source of truth").
func resolveAnthropicFromAuthStore(ctx context.Context, app string) error {
	if app == "" {
		return fmt.Errorf("auth-store app name is empty")
	}

	base := strings.TrimRight(envOr("AUTH_STORE_URL", "http://127.0.0.1:8303"), "/")
	token := os.Getenv("AUTH_STORE_TOKEN")
	if token == "" {
		return fmt.Errorf("AUTH_STORE_TOKEN env var is required when --api-key-from-auth-store is set")
	}

	u := base + "/api/resolve/anthropic"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("build resolve request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Auth-App", app)
	req.Header.Set("X-Auth-Reason", "inber-server startup")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("auth-store resolve: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth-store resolve: %s — %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var r struct {
		AuthType    string `json:"auth_type"`
		APIKey      string `json:"api_key"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("decode resolve response: %w", err)
	}

	var secret string
	switch {
	case r.APIKey != "":
		secret = r.APIKey
	case r.AccessToken != "":
		secret = r.AccessToken
	default:
		return fmt.Errorf("auth-store: anthropic credential has no api_key or access_token (auth_type=%q)", r.AuthType)
	}

	if err := os.Setenv("ANTHROPIC_API_KEY", secret); err != nil {
		return fmt.Errorf("set ANTHROPIC_API_KEY: %w", err)
	}
	logger.WithComponent("auth-store").Info("resolved anthropic credential", map[string]interface{}{
		"app":       app,
		"auth_type": r.AuthType,
	})
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
