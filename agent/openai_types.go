package agent

// OpenAI API types (exported for use in engine)

type OpenAIMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content,omitempty"` // string or []contentPart
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"` // always "function"
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type OpenAITool struct {
	Type     string               `json:"type"` // always "function"
	Function OpenAIFunctionSchema `json:"function"`
}

type OpenAIFunctionSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type OpenAIRequest struct {
	Model                string          `json:"model"`
	Messages             []OpenAIMessage `json:"messages"`
	Tools                []OpenAITool    `json:"tools,omitempty"`
	Temperature          float64         `json:"temperature,omitempty"`
	MaxTokens            int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens  int             `json:"max_completion_tokens,omitempty"`
	Stream               bool            `json:"stream,omitempty"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// PromptTokensDetails breaks PromptTokens down. Every OpenAI-compatible
	// endpoint that caches reports the cached part here and nowhere else, so
	// leaving it off the struct dropped it at the JSON boundary: nothing
	// downstream could see it, and no measurement of this path's cache
	// behaviour was possible even in principle.
	PromptTokensDetails OpenAIPromptTokensDetails `json:"prompt_tokens_details"`
}

// OpenAIPromptTokensDetails is the breakdown of OpenAIUsage.PromptTokens.
type OpenAIPromptTokensDetails struct {
	// CachedTokens is the part of PromptTokens the provider served from its own
	// cache. It is a SUBSET of PromptTokens, not a figure beside it — which is
	// the opposite of Anthropic's convention, where input_tokens excludes the
	// cached span and cache_read_input_tokens counts it separately. Anything
	// that prices this number has to say which of the two conventions it is
	// working in; see agent.APICallUsage.CachedTokensIncludedInInputTokens.
	CachedTokens int `json:"cached_tokens"`
}
