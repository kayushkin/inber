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
}