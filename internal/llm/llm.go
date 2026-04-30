package llm

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrInvalidResponse is returned when the provider reply doesn't match the expected schema.
var ErrInvalidResponse = errors.New("llm: invalid response")

// Message is a single turn in a conversation.
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Request is the input to Provider.Complete.
type Request struct {
	Messages    []Message
	JSONSchema  json.RawMessage // structured output schema; nil = plain text
	Model       string          // empty = use provider default
	MaxTokens   int
	Temperature float32
}

// Response is the output from Provider.Complete.
type Response struct {
	Content      string
	Model        string
	InputTokens  int
	OutputTokens int
	FinishReason string
	LatencyMs    int
}

// Provider is the LLM abstraction used by all callers.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}
