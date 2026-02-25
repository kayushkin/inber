package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
)

// props builds an InputSchema from a map of property definitions.
func props(required []string, properties map[string]any) anthropic.ToolInputSchemaParam {
	s := anthropic.ToolInputSchemaParam{
		Properties: properties,
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func number(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func array(desc, itemType string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": itemType},
	}
}

// SearchTool returns a tool for searching memories.
func SearchTool(store *Store) agent.Tool {
	type input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	return agent.Tool{
		Name:        "memory_search",
		Description: "Search persistent memories by semantic similarity to a query. Returns relevant memories ranked by similarity, importance, and recency.",
		InputSchema: props([]string{"query"}, map[string]any{
			"query": str("Search query text"),
			"limit": integer("Maximum number of results to return (default: 10)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}
			if in.Limit == 0 {
				in.Limit = 10
			}

			memories, err := store.Search(in.Query, in.Limit)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}

			if len(memories) == 0 {
				return "No memories found matching the query.", nil
			}

			var lines []string
			for i, m := range memories {
				tags := strings.Join(m.Tags, ", ")
				lines = append(lines, fmt.Sprintf(
					"%d. [%s] (importance: %.2f, accessed: %d times, source: %s)\n   Tags: %s\n   %s",
					i+1, m.ID, m.Importance, m.AccessCount, m.Source, tags, m.Content,
				))
			}
			return strings.Join(lines, "\n\n"), nil
		},
	}
}

// SaveTool returns a tool for saving new memories.
func SaveTool(store *Store) agent.Tool {
	type input struct {
		Content    string   `json:"content"`
		Tags       []string `json:"tags"`
		Importance float64  `json:"importance"`
		Source     string   `json:"source"`
	}
	return agent.Tool{
		Name:        "memory_save",
		Description: "Store a new memory for persistent recall across sessions. Memories are automatically embedded for semantic search.",
		InputSchema: props([]string{"content"}, map[string]any{
			"content":    str("The memory content to store"),
			"tags":       array("Tags for categorization (e.g., 'code', 'preference', 'fact')", "string"),
			"importance": number("Importance score 0-1 (default: 0.5). Higher scores = higher priority in search."),
			"source":     str("Source of the memory: 'user', 'agent', 'system' (default: 'agent')"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			if in.Content == "" {
				return "error: content cannot be empty", nil
			}

			// Set defaults
			if in.Importance == 0 {
				in.Importance = 0.5
			}
			if in.Importance < 0 || in.Importance > 1 {
				return "error: importance must be between 0 and 1", nil
			}
			if in.Source == "" {
				in.Source = "agent"
			}

			m := Memory{
				ID:         uuid.New().String(),
				Content:    in.Content,
				Tags:       in.Tags,
				Importance: in.Importance,
				Source:     in.Source,
			}

			if err := store.Save(m); err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}

			return fmt.Sprintf("Memory saved with ID: %s", m.ID), nil
		},
	}
}

// ExpandTool returns a tool for retrieving detailed memory content.
func ExpandTool(store *Store) agent.Tool {
	type input struct {
		ID string `json:"id"`
	}
	return agent.Tool{
		Name:        "memory_expand",
		Description: "Retrieve the full content of a memory by ID. Useful for expanding compacted summaries or revisiting specific memories.",
		InputSchema: props([]string{"id"}, map[string]any{
			"id": str("Memory ID to retrieve"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			m, err := store.Get(in.ID)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}

			tags := strings.Join(m.Tags, ", ")
			result := fmt.Sprintf(
				"Memory ID: %s\nSource: %s\nCreated: %s\nLast accessed: %s\nAccess count: %d\nImportance: %.2f\nTags: %s\n\nContent:\n%s",
				m.ID, m.Source, m.CreatedAt.Format("2006-01-02 15:04:05"),
				m.LastAccessed.Format("2006-01-02 15:04:05"),
				m.AccessCount, m.Importance, tags, m.Content,
			)

			if m.OriginalID != "" {
				result += fmt.Sprintf("\n\n(This memory is a compacted summary of memory %s)", m.OriginalID)
			}

			return result, nil
		},
	}
}

// ForgetTool returns a tool for marking memories as forgotten.
func ForgetTool(store *Store) agent.Tool {
	type input struct {
		ID string `json:"id"`
	}
	return agent.Tool{
		Name:        "memory_forget",
		Description: "Mark a memory as forgotten/irrelevant. This is a soft delete — the memory remains in storage but won't appear in search results.",
		InputSchema: props([]string{"id"}, map[string]any{
			"id": str("Memory ID to forget"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			if err := store.Forget(in.ID); err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}

			return fmt.Sprintf("Memory %s has been forgotten.", in.ID), nil
		},
	}
}

// AllMemoryTools returns all memory tools for the given store.
func AllMemoryTools(store *Store) []agent.Tool {
	return []agent.Tool{
		SearchTool(store),
		SaveTool(store),
		ExpandTool(store),
		ForgetTool(store),
	}
}
