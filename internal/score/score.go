package score

import (
	"context"
	"math"
	"time"

	"github.com/Tragidra/loglens/internal/config"
	"github.com/Tragidra/loglens/model"
)

// WindowStats holds aggregated metrics for one scoring window.
type WindowStats struct {
	CountNow         int64
	CountAvgRecent   float64
	CountTotal       int64
	ServicesInWindow int
	Level            model.Level
	FirstSeen        time.Time
}

// Scorer assigns a priority score (0-100) and flags to a cluster.
type Scorer interface {
	Score(ctx context.Context, c *model.Cluster, window WindowStats) (int, []string)
}

type scorer struct {
	cfg config.ScoreConfig
}

// New returns a Scorer using the provided config.
func New(cfg config.ScoreConfig) Scorer {
	return &scorer{cfg: cfg}
}

func (s *scorer) Score(_ context.Context, c *model.Cluster, w WindowStats) (int, []string) {
	sc := 0.0
	var flags []string

	switch w.Level {
	case model.LevelFatal:
		sc += float64(s.cfg.Weights.LevelFatal)
	case model.LevelError:
		sc += float64(s.cfg.Weights.LevelError)
	case model.LevelWarn:
		sc += float64(s.cfg.Weights.LevelWarn)
	case model.LevelInfo:
		sc += float64(s.cfg.Weights.LevelInfo)
	}

	if w.CountNow > 0 {
		sc += s.cfg.Weights.Frequency * math.Log2(float64(w.CountNow)+1)
	}

	if w.CountAvgRecent > 0 {
		ratio := float64(w.CountNow) / w.CountAvgRecent
		if ratio >= 3.0 {
			sc += s.cfg.Weights.Burst * math.Min(ratio/3.0, 5.0)
			flags = append(flags, "burst")
		}
	}

	if time.Since(w.FirstSeen) < 10*time.Minute {
		sc += s.cfg.Weights.Novelty
		flags = append(flags, "novel")
	}

	if w.CountTotal < 5 {
		sc += s.cfg.Weights.Rarity
		flags = append(flags, "rare")
	}

	if w.ServicesInWindow >= 2 {
		sc += s.cfg.Weights.CrossService
		flags = append(flags, "cross-service")
	}

	if sc < 0 {
		sc = 0
	}
	if sc > 100 {
		sc = 100
	}
	return int(math.Round(sc)), flags
}
