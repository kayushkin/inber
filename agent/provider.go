package agent

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
)

// Provider defines the interface for LLM providers that can complete messages.
// This abstraction allows swapping between different providers (Anthropic, OpenAI, local models)
// without changing the agent logic.
type Provider interface {
	// Complete sends a message request and returns the complete response.
	// This is used for non-streaming requests.
	Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error)
	
	// CompleteStreaming sends a message request and returns a streaming response.
	// This is used when OnTextDelta hooks are configured.
	CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error)
}

// StreamingResponse provides an interface for handling streaming message responses.
type StreamingResponse interface {
	// Next advances to the next event in the stream.
	// Returns false when the stream is exhausted.
	Next() bool
	
	// Current returns the current streaming event.
	Current() anthropic.MessageStreamEventUnion
	
	// Err returns any error that occurred during streaming.
	Err() error
}

// AnthropicProvider implements the Provider interface using the Anthropic SDK.
type AnthropicProvider struct {
	client *anthropic.Client
}

// NewAnthropicProvider creates a new Anthropic provider with the given client.
func NewAnthropicProvider(client *anthropic.Client) *AnthropicProvider {
	return &AnthropicProvider{
		client: client,
	}
}

// Complete implements the Provider interface for Anthropic.
func (p *AnthropicProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return p.client.Messages.New(ctx, *params)
}

// CompleteStreaming implements the Provider interface for Anthropic.
func (p *AnthropicProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	stream := p.client.Messages.NewStreaming(ctx, *params)
	return &anthropicStreamingResponse{stream: stream}, nil
}

// anthropicStreamingResponse wraps the Anthropic streaming response to implement StreamingResponse.
type anthropicStreamingResponse struct {
	stream interface {
		Next() bool
		Current() anthropic.MessageStreamEventUnion
		Err() error
	}
}

func (r *anthropicStreamingResponse) Next() bool {
	return r.stream.Next()
}

func (r *anthropicStreamingResponse) Current() anthropic.MessageStreamEventUnion {
	return r.stream.Current()
}

func (r *anthropicStreamingResponse) Err() error {
	return r.stream.Err()
}