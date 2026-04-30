// Package analyze sends high-priority clusters to an LLM provider and stores the resulting analyses.
// The Analyzer interface has a single method, Analyze, which accepts a cluster ID,
// builds a prompt from cluster metadata and recent examples, calls the provider, validates the structured JSON response,
// and persists the result.
// Responses are cached by (cluster fingerprint, time window) to avoid re-billing
// on repeated calls within the same window.
//
// WorkerPool runs Analyze calls off the hot path so ingestion is never blocked waiting for an LLM round-trip.
package analyze
