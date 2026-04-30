// Package ingest defines the Source interface for log ingestion.
package ingest

import (
	"context"

	"github.com/Tragidra/logsense/model"
)

// Source is a single named ingestion endpoint.
type Source interface {
	Name() string
	Stream(ctx context.Context, out chan<- model.RawLog) error
}
