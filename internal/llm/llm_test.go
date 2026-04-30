package llm_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tragidra/loglens/internal/config"
	"github.com/Tragidra/loglens/internal/llm"
	"github.com/Tragidra/loglens/internal/llm/fake"
	"github.com/Tragidra/loglens/internal/llm/openrouter"
)

// for fake ai-provider
func TestFake_ReturnsConfiguredResponse(t *testing.T) {
	p := fake.New()
	p.SetResponse(llm.Response{Content: `{"result":"ok"}`, InputTokens: 10, OutputTokens: 5})

	resp, err := p.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	assert.Equal(t, `{"result":"ok"}`, resp.Content)
	assert.Equal(t, 10, resp.InputTokens)
}

func TestFake_ReturnsConfiguredError(t *testing.T) {
	p := fake.New()
	p.SetError(llm.ErrInvalidResponse)

	_, err := p.Complete(context.Background(), llm.Request{})
	assert.ErrorIs(t, err, llm.ErrInvalidResponse)
}

func TestFake_RecordsCalls(t *testing.T) {
	p := fake.New()
	req := llm.Request{
		Messages:  []llm.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "msg"}},
		MaxTokens: 256,
	}
	_, _ = p.Complete(context.Background(), req)
	_, _ = p.Complete(context.Background(), req)

	calls := p.Calls()
	assert.Len(t, calls, 2)
	assert.Equal(t, "sys", calls[0].Messages[0].Content)
}

func TestOpenRouter_RequestPayload(t *testing.T) {
	var received map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "https://github.com/lux/loglens", r.Header.Get("HTTP-Referer"))
		assert.Equal(t, "LogLens", r.Header.Get("X-Title"))

		json.NewDecoder(r.Body).Decode(&received)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"role": "assistant", "content": "pong"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 12, "completion_tokens": 3},
		})
	}))
	defer srv.Close()

	p, err := openrouter.New(&config.LLMConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "openai/gpt-4o-mini",
	}, noopLogger(t))
	require.NoError(t, err)

	schema := json.RawMessage(`{"type":"object"}`)
	resp, err := p.Complete(context.Background(), llm.Request{
		Messages:    []llm.Message{{Role: "user", Content: "ping"}},
		Model:       "openai/gpt-4o-mini",
		MaxTokens:   100,
		Temperature: 0.2,
		JSONSchema:  schema,
	})
	require.NoError(t, err)
	assert.Equal(t, "pong", resp.Content)
	assert.Equal(t, 12, resp.InputTokens)
	assert.Equal(t, 3, resp.OutputTokens)

	assert.Equal(t, "openai/gpt-4o-mini", received["model"])
	msgs := received["messages"].([]interface{})
	assert.Len(t, msgs, 1)
	assert.Equal(t, "ping", msgs[0].(map[string]interface{})["content"])

	rf := received["response_format"].(map[string]interface{})
	assert.Equal(t, "json_schema", rf["type"])
}

func TestOpenRouter_HTTP500Retried(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, err := openrouter.New(&config.LLMConfig{
		APIKey:     "key",
		BaseURL:    srv.URL,
		MaxRetries: 2,
	}, noopLogger(t))
	require.NoError(t, err)

	_, err = p.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Equal(t, 3, attempts, "should have tried 3 times (1 + 2 retries)")
}

func TestOpenRouter_HTTP400NotRetried(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	p, err := openrouter.New(&config.LLMConfig{
		APIKey:     "key",
		BaseURL:    srv.URL,
		MaxRetries: 3,
	}, noopLogger(t))
	require.NoError(t, err)

	_, err = p.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Equal(t, 1, attempts, "4xx should not be retried")
}

func TestOpenRouterReal(t *testing.T) {
	t.Skip("skipping: run with TEST_LLM_REAL=1 LLM_API_KEY=... go test -run TestOpenRouterReal -v")
}

func noopLogger(_ *testing.T) *slog.Logger {
	return slog.Default()
}
