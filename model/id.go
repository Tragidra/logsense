package model

import (
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropyMu sync.Mutex
	// Monotonic entropy  guarantees ordering within the same millisecond.
	entropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
)

// NewID returns a new time-ordered ID string.
// IDs generated within the same millisecond are guaranteed to be increasing.
func NewID() string {
	entropyMu.Lock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	entropyMu.Unlock()
	return id.String()
}
