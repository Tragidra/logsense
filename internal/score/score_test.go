package score

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Tragidra/loglens/internal/config"
	"github.com/Tragidra/loglens/model"
)

func defaultWeights() config.ScoreWeights {
	return config.ScoreWeights{
		LevelFatal:   40,
		LevelError:   25,
		LevelWarn:    10,
		LevelInfo:    0,
		Frequency:    1.0,
		Burst:        15,
		Novelty:      20,
		Rarity:       10,
		CrossService: 15,
	}
}

func newScorer() Scorer {
	return New(config.ScoreConfig{Weights: defaultWeights()})
}

var ctx = context.Background()

func TestScore_FatalNovelCrossService(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:         1,
		CountAvgRecent:   0,
		CountTotal:       1,
		ServicesInWindow: 2,
		Level:            model.LevelFatal,
		FirstSeen:        time.Now().Add(-1 * time.Minute),
	}
	priority, flags := s.Score(ctx, c, w)
	// 40 (fatal) + 20 (novel) + 10 (rare) + 15 (cross-service) + log2(2)≈1 = 86
	assert.GreaterOrEqual(t, priority, 80)
	assert.Contains(t, flags, "novel")
	assert.Contains(t, flags, "rare")
	assert.Contains(t, flags, "cross-service")
}

func TestScore_SteadyInfo(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:         100,
		CountAvgRecent:   95,
		CountTotal:       10000,
		ServicesInWindow: 1,
		Level:            model.LevelInfo,
		FirstSeen:        time.Now().Add(-24 * time.Hour),
	}
	priority, flags := s.Score(ctx, c, w)
	// 0 (info) + log2(101)≈6.6 = ~7, no burst/novel/rare/cross-service
	assert.Less(t, priority, 20)
	assert.Empty(t, flags)
}

func TestScore_BurstFlag(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:         90,
		CountAvgRecent:   10,
		CountTotal:       500,
		ServicesInWindow: 1,
		Level:            model.LevelError,
		FirstSeen:        time.Now().Add(-1 * time.Hour),
	}
	_, flags := s.Score(ctx, c, w)
	assert.Contains(t, flags, "burst")
}

func TestScore_NoBurstBelowThreshold(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:       20,
		CountAvgRecent: 15, // ratio = 1.33, below 3.0
		CountTotal:     100,
		Level:          model.LevelWarn,
		FirstSeen:      time.Now().Add(-1 * time.Hour),
	}
	_, flags := s.Score(ctx, c, w)
	for _, f := range flags {
		assert.NotEqual(t, "burst", f)
	}
}

func TestScore_RareFlag(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:   1,
		CountTotal: 4, // < 5 → rare
		Level:      model.LevelInfo,
		FirstSeen:  time.Now().Add(-1 * time.Hour),
	}
	_, flags := s.Score(ctx, c, w)
	assert.Contains(t, flags, "rare")
}

func TestScore_ClipsAt100(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	w := WindowStats{
		CountNow:         1000,
		CountAvgRecent:   1,
		CountTotal:       1,
		ServicesInWindow: 5,
		Level:            model.LevelFatal,
		FirstSeen:        time.Now(),
	}
	priority, _ := s.Score(ctx, c, w)
	assert.LessOrEqual(t, priority, 100)
}

func TestScore_ClipsAt0(t *testing.T) {
	s := New(config.ScoreConfig{Weights: config.ScoreWeights{
		LevelInfo: -100,
	}})
	c := &model.Cluster{}
	w := WindowStats{Level: model.LevelInfo, FirstSeen: time.Now().Add(-1 * time.Hour), CountTotal: 100}
	priority, _ := s.Score(ctx, c, w)
	assert.GreaterOrEqual(t, priority, 0)
}

func TestScore_LevelMonotonicity(t *testing.T) {
	s := newScorer()
	c := &model.Cluster{}
	base := WindowStats{
		CountNow:       10,
		CountAvgRecent: 10,
		CountTotal:     100,
		FirstSeen:      time.Now().Add(-1 * time.Hour),
	}

	score := func(level model.Level) int {
		w := base
		w.Level = level
		p, _ := s.Score(ctx, c, w)
		return p
	}

	assert.Greater(t, score(model.LevelFatal), score(model.LevelError))
	assert.Greater(t, score(model.LevelError), score(model.LevelWarn))
	assert.Greater(t, score(model.LevelWarn), score(model.LevelInfo))
}

func TestWindowBuffer_Record(t *testing.T) {
	b := NewWindowBuffer()
	b.Record("svc-a", model.LevelError)
	b.Record("svc-b", model.LevelWarn)

	countNow, _, services, dominant, total := b.Snapshot()
	assert.Equal(t, int64(2), countNow)
	assert.Equal(t, 2, services)
	assert.Equal(t, model.LevelError, dominant)
	assert.Equal(t, int64(2), total)
}

func TestWindowBuffer_AdvancesSlot(t *testing.T) {
	b := NewWindowBuffer()
	b.Record("svc", model.LevelInfo)
	_, _, _, _, _ = b.Snapshot()

	countNow, _, _, _, _ := b.Snapshot()
	assert.Equal(t, int64(0), countNow)
}

func TestWindowBuffer_AvgRecent(t *testing.T) {
	b := NewWindowBuffer()
	for i := 0; i < 4; i++ {
		for j := 0; j < 10; j++ {
			b.Record("svc", model.LevelInfo)
		}
		b.Snapshot()
	}
	for i := 0; i < 30; i++ {
		b.Record("svc", model.LevelInfo)
	}
	countNow, avgRecent, _, _, _ := b.Snapshot()
	assert.Equal(t, int64(30), countNow)
	assert.InDelta(t, 10.0, avgRecent, 1.0)
}

func TestWindowBuffer_TotalAccumulates(t *testing.T) {
	b := NewWindowBuffer()
	for i := 0; i < 7; i++ {
		b.Record("svc", model.LevelInfo)
	}
	b.Snapshot()
	for i := 0; i < 3; i++ {
		b.Record("svc", model.LevelInfo)
	}
	_, _, _, _, total := b.Snapshot()
	assert.Equal(t, int64(10), total)
}
