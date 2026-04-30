// Package storage defines the repository interface used by the rest of LogLens, implementations live in subpackages
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/Tragidra/loglens/model"
)

// ErrNotImplemented is returned by stub Repository implementations for methods that are not needed by
// a particular caller.
var ErrNotImplemented = errors.New("storage: not implemented")

// ErrNotFound is returned when a lookup by primary key yields no rows.
var ErrNotFound = errors.New("storage: not found")

// Repository is the persistence boundary for the rest of the application.
type Repository interface {
	// Events
	SaveEvent(ctx context.Context, e model.LogEvent, clusterID string) error
	ListEventsByCluster(ctx context.Context, clusterID string, filter EventFilter) ([]model.LogEvent, error)

	// Clusters
	UpsertCluster(ctx context.Context, c model.Cluster) error
	GetCluster(ctx context.Context, id string) (model.Cluster, error)
	ListClusters(ctx context.Context, filter ClusterFilter) ([]model.Cluster, int64, error)
	PruneStaleClusters(ctx context.Context, olderThan time.Time) (int64, error)

	// Analyses
	SaveAnalysis(ctx context.Context, a model.Analysis) error
	LatestAnalysisForCluster(ctx context.Context, clusterID string) (*model.Analysis, error)
	ListRecentAnalyses(ctx context.Context, limit int) ([]model.Analysis, error)

	// lifecycle
	Ping(ctx context.Context) error
	Close() error
}

// ClusterFilter decrease ListClusters results.
type ClusterFilter struct {
	From           *time.Time
	To             *time.Time
	MinPriority    *int
	Services       []string
	Levels         []model.Level
	SearchTemplate string
	Limit          int
	Offset         int
	OrderBy        string
}

// EventFilter decrease ListEventsByCluster results.
type EventFilter struct {
	From   *time.Time
	To     *time.Time
	Levels []model.Level
	Limit  int
	Offset int
}
