package score

import (
	"sync"

	"github.com/Tragidra/logstruct/model"
)

const windowSlots = 5

type windowSlot struct {
	count    int64
	services map[string]struct{}
	level    model.Level
}

// WindowBuffer is a ring buffer that tracks per-cluster event counts across the last windowSlots scoring windows.
type WindowBuffer struct {
	mu    sync.Mutex
	slots [windowSlots]windowSlot
	cur   int
	total int64
}

// NewWindowBuffer returns an initialised WindowBuffer.
func NewWindowBuffer() *WindowBuffer {
	b := &WindowBuffer{}
	for i := range b.slots {
		b.slots[i].services = make(map[string]struct{})
	}
	return b
}

// Record adds one event to the current slot.
func (b *WindowBuffer) Record(service string, level model.Level) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total++
	s := &b.slots[b.cur]
	s.count++
	if service != "" {
		s.services[service] = struct{}{}
	}
	if level > s.level {
		s.level = level
	}
}

// Snapshot returns the current window stats and advances to the next slot (zeroing it ready for recording).
func (b *WindowBuffer) Snapshot() (countNow int64, avgRecent float64, services int, dominant model.Level, total int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cur := &b.slots[b.cur]
	countNow = cur.count
	dominant = cur.level

	services = len(cur.services)

	var sum int64
	filled := 0
	for i := 1; i < windowSlots; i++ {
		idx := (b.cur - i + windowSlots) % windowSlots
		if b.slots[idx].count > 0 {
			sum += b.slots[idx].count
			filled++
		}
	}
	if filled > 0 {
		avgRecent = float64(sum) / float64(filled)
	}

	total = b.total

	b.cur = (b.cur + 1) % windowSlots
	next := &b.slots[b.cur]
	next.count = 0
	next.level = 0
	next.services = make(map[string]struct{})

	return
}
