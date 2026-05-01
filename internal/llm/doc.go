// Package llm defines the Provider interface and shared types used by all LLM provider implementations.
//
// All providers live in subpackages:
//   - llm/logstructai: local OpenAI-compatible server (LM Studio, Ollama, etc.)
//   - llm/openrouter: OpenRouter hosted models
//   - llm/fake: in-memory stub for tests
//
// WithRetry wraps a provider call with configurable retries, returning on the first non-retryable error.
package llm
