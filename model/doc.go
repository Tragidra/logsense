// Package model defines the domain types used by logsense and returned by lib public API.
// The main types are:
//   - RawLog: a raw line as received from a source before normalization.
//   - LogEvent: a normalized log entry.
//   - Cluster: a group of LogEvents sharing the same template.
//   - Analysis: an LLM-generated explanation for a Cluster.
//   - Severity: info | warning | critical, as assessed by the LLM.
//   - Level: debug | info | warn | error | fatal, parsed from the log line.
package model
