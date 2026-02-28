package context

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// RepoMapToolInput defines the input schema for the repo_map tool
type RepoMapToolInput struct {
	Path string `json:"path,omitempty"` // Optional: specific subdirectory to map (e.g., "agent/")
}

// RepoMapTool creates an agent tool that generates repository structure maps on demand.
// This replaces the automatic repo map loading in the system prompt, saving ~9400 tokens
// until the agent actually needs to understand project structure.
func RepoMapTool(rootDir string, ignorePatterns []string) agent.Tool {
	return agent.Tool{
		Name:        "repo_map",
		Description: "Generate a structural map of the repository showing Go packages, functions, types, and important files. Use this when you need to understand project structure, find where code lives, or explore unfamiliar parts of the codebase. Optionally specify a subdirectory path to map only that subtree (e.g., 'agent/' or 'context/').",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Optional subdirectory to map (e.g., 'agent/', 'context/'). If not specified, maps entire repository.",
				},
			},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			var params RepoMapToolInput
			if err := json.Unmarshal([]byte(input), &params); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}

			// Determine the target directory
			targetDir := rootDir
			if params.Path != "" {
				// Clean the path and make it relative
				cleanPath := filepath.Clean(params.Path)
				targetDir = filepath.Join(rootDir, cleanPath)
				
				// Verify the path exists and is within rootDir
				if !strings.HasPrefix(targetDir, rootDir) {
					return "", fmt.Errorf("path must be within repository root")
				}
			}

			// Build the repo map
			repoMap, err := BuildRepoMap(targetDir, ignorePatterns)
			if err != nil {
				return "", fmt.Errorf("failed to build repo map: %w", err)
			}

			// Add a header indicating what was mapped
			var result strings.Builder
			if params.Path != "" {
				result.WriteString(fmt.Sprintf("# Repository Structure: %s\n\n", params.Path))
			} else {
				result.WriteString("# Repository Structure\n\n")
			}
			result.WriteString(repoMap)

			return result.String(), nil
		},
	}
}
