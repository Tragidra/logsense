package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Tragidra/logsense/internal/config"
	"github.com/Tragidra/logsense/internal/llm"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	httpReferer    = "https://github.com/Tragidra/logsense"
	xTitle         = "logsense"
)

// Provider implements llm.Provider against the OpenRouter API.
type Provider struct {
	cfg    *config.LLMConfig
	client *http.Client
	logger *slog.Logger
}

// New constructs an OpenRouter Provider.
func New(cfg *config.LLMConfig, logger *slog.Logger) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openrouter: api_key is required")
	}
	timeout := cfg.Timeout.D()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}, nil
}

func (p *Provider) Name() string { return "openrouter" }

// Complete sends a chat completion request to OpenRouter and returns the response.
func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	maxRetries := p.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return llm.WithRetry(ctx, maxRetries, func() (llm.Response, *llm.HTTPStatusError, error) {
		return p.do(ctx, req)
	})
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float32         `json:"temperature,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *Provider) do(ctx context.Context, req llm.Request) (llm.Response, *llm.HTTPStatusError, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.cfg.MaxTokens
	}
	temp := req.Temperature
	if temp <= 0 {
		temp = float32(p.cfg.Temperature)
	}

	msgs := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMessage{Role: m.Role, Content: m.Content}
	}

	cr := chatRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: temp,
	}
	if len(req.JSONSchema) > 0 {
		cr.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: req.JSONSchema,
		}
	}

	body, err := json.Marshal(cr)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}

	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("openrouter: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("HTTP-Referer", httpReferer)
	httpReq.Header.Set("X-Title", xTitle)

	start := time.Now()
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("openrouter: http: %w", err)
	}
	defer httpResp.Body.Close()
	latency := int(time.Since(start).Milliseconds())

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("openrouter: read body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return llm.Response{}, &llm.HTTPStatusError{StatusCode: httpResp.StatusCode, Body: snippet}, nil
	}

	var out chatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return llm.Response{}, nil, fmt.Errorf("openrouter: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return llm.Response{}, nil, llm.ErrInvalidResponse
	}

	resp := llm.Response{
		Content:      out.Choices[0].Message.Content,
		Model:        out.Model,
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
		FinishReason: out.Choices[0].FinishReason,
		LatencyMs:    latency,
	}

	p.logger.Info("llm request",
		"provider", "openrouter",
		"model", resp.Model,
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
		"latency_ms", resp.LatencyMs,
	)

	return resp, nil, nil
}
