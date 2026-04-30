// Package api implements the HTTP server that the loglens UI and external consumers read from.
//
// The server is read-only from the perspective of log data — it never ingests events. Routes are mounted under /api/:
//
//	GET  /api/healthz              liveness probe
//	GET  /api/readyz               readiness probe (pings storage)
//	GET  /api/clusters             list clusters with optional filters
//	GET  /api/clusters/{id}        cluster detail
//	GET  /api/clusters/{id}/events events for a cluster
//	POST /api/clusters/{id}/analyze trigger an LLM analysis for a cluster
package api
