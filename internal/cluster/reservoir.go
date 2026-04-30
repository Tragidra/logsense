package cluster

import (
	"math/rand"
	"sync"
	"time"
)

// Reservoir holds a random sample of strings, it uses Algorithm R
// In short, then each new item replaces a random existing item with probability cap/n
type Reservoir struct {
	mu    sync.Mutex
	items []string
	cap   int
	count int64
	rng   *rand.Rand
}

// NewReservoir returns a Reservoir that keeps up to capacity items.
func NewReservoir(capacity int) *Reservoir {
	return &Reservoir{
		items: make([]string, 0, capacity),
		cap:   capacity,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Add inserts s into the reservoir sample.
func (r *Reservoir) Add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	if len(r.items) < r.cap {
		r.items = append(r.items, s)
		return
	}
	j := r.rng.Int63n(r.count)
	if j < int64(r.cap) {
		r.items[j] = s
	}
}

// Items returns a copy of the current sample.
func (r *Reservoir) Items() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.items))
	copy(out, r.items)
	return out
}
