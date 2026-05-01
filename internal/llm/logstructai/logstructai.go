// Package logstructai implements llm.Provider for a local OpenAI-compatible local server (LM Studio, for example).
//
// Default endpoint is http://localhost:1234/v1 (LM Studio's default for 04.2026). No API key is required
// when talking to localhost. Uses json_schema response_format when a schema is provided.
package logstructai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Tragidra/logstruct/internal/config"
	"github.com/Tragidra/logstruct/internal/llm"
)

const defaultBaseURL = "http://localhost:1234/v1"

// Provider implements llm.Provider against a local OpenAI-compatible server.
type Provider struct {
	cfg    *config.LLMConfig
	client *http.Client
	logger *slog.Logger
}

// New constructs a logstruct-ai Provider. API key is optional.
func New(cfg *config.LLMConfig, logger *slog.Logger) (*Provider, error) {
	timeout := cfg.Timeout.D()
	if timeout <= 0 {
		timeout = 60 * time.Second // local models can be slow
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	logger.Info("logstruct-ai: provider initialized",
		"base_url", baseURL,
		"model", cfg.Model,
		"timeout", timeout,
		"response_format", "json_schema (with text fallback on 400)",
	)
	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}, nil
}

func (p *Provider) Name() string { return "logstruct-ai" }

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	maxRetries := p.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	resp, err := llm.WithRetry(ctx, maxRetries, func() (llm.Response, *llm.HTTPStatusError, error) {
		return p.do(ctx, req)
	})
	if err == nil {
		return resp, nil
	}
	// If the model rejected response_format (400), retry without it, the prompt instructs the model to return JSON.
	if isFormatError(err) && len(req.JSONSchema) > 0 {
		p.logger.Warn("logstruct-ai: response_format rejected, retrying without it", "err", err)
		noSchema := req
		noSchema.JSONSchema = nil
		return llm.WithRetry(ctx, maxRetries, func() (llm.Response, *llm.HTTPStatusError, error) {
			return p.do(ctx, noSchema)
		})
	}
	return resp, err
}

func isFormatError(err error) bool {
	var httpErr *llm.HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 400
	}
	return false
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
	// The prompt already contains the schema description as a fallback for models that ignore response_format.
	if len(req.JSONSchema) > 0 {
		cr.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: req.JSONSchema,
		}
	}

	body, err := json.Marshal(cr)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("logstruct-ai: marshal request: %w", err)
	}

	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	respFormatType := "none"
	if cr.ResponseFormat != nil {
		respFormatType = cr.ResponseFormat.Type
	}
	p.logger.Debug("logstruct-ai: sending request",
		"url", baseURL+"/chat/completions",
		"model", model,
		"response_format_type", respFormatType,
		"body_bytes", len(body),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("logstruct-ai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	start := time.Now()
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("logstruct-ai: http: %w", err)
	}
	defer httpResp.Body.Close()
	latency := int(time.Since(start).Milliseconds())

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, nil, fmt.Errorf("logstruct-ai: read body: %w", err)
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
		return llm.Response{}, nil, fmt.Errorf("logstruct-ai: decode response: %w", err)
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
		"provider", "logstruct-ai",
		"model", resp.Model,
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
		"latency_ms", resp.LatencyMs,
	)

	return resp, nil, nil
}
