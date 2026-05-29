// Package store defines the storage interface attribution events land in.
//
// Phase 0 ships an in-memory implementation. Phase 1 adds SQLite (via
// modernc.org/sqlite, pure-Go to keep CGO out of the build) and
// Postgres. The interface is deliberately narrow — Insert + Query — so
// adding a backend is a couple-hundred-line drop-in, not a refactor.
package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
)

// Store is the persistence interface. Implementations are safe for
// concurrent use.
type Store interface {
	// Insert persists one event. Returns the event id assigned by the
	// store (UUIDv7-shaped) and any error.
	Insert(ctx context.Context, e ingest.Event) (string, error)

	// Query returns events matching the filter, sorted by Timestamp asc.
	Query(ctx context.Context, f Filter) ([]ingest.Event, error)

	// Count returns the matching row count without materializing events.
	Count(ctx context.Context, f Filter) (int, error)
}

// Filter constrains a Query/Count. Empty fields are wildcards.
type Filter struct {
	Source        string
	Medium        string
	Campaign      string
	Surface       string
	ConsentStatus string
	Since         time.Time // inclusive lower bound on Timestamp
	Until         time.Time // exclusive upper bound on Timestamp
	Limit         int       // 0 means unlimited
}

// ─── in-memory implementation ───────────────────────────────────────────

// Memory is an in-memory Store backed by a slice. Adequate for Phase 0
// + tests + small deployments. Concurrency-safe.
type Memory struct {
	mu     sync.RWMutex
	events []indexed
	nextID int64
}

type indexed struct {
	ID    string
	Event ingest.Event
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory {
	return &Memory{}
}

func (m *Memory) Insert(ctx context.Context, e ingest.Event) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := generateID(m.nextID)
	m.events = append(m.events, indexed{ID: id, Event: e})
	return id, nil
}

func (m *Memory) Query(ctx context.Context, f Filter) ([]ingest.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ingest.Event, 0, len(m.events))
	for _, item := range m.events {
		if matches(f, item.Event) {
			out = append(out, item.Event)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp < out[j].Timestamp
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (m *Memory) Count(ctx context.Context, f Filter) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, item := range m.events {
		if matches(f, item.Event) {
			n++
		}
	}
	return n, nil
}

func matches(f Filter, e ingest.Event) bool {
	if f.Source != "" && e.Source != f.Source {
		return false
	}
	if f.Medium != "" && e.Medium != f.Medium {
		return false
	}
	if f.Campaign != "" && e.Campaign != f.Campaign {
		return false
	}
	if f.Surface != "" && e.Surface != f.Surface {
		return false
	}
	if f.ConsentStatus != "" && e.ConsentStatus != f.ConsentStatus {
		return false
	}
	if !f.Since.IsZero() || !f.Until.IsZero() {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			// Events with malformed timestamps fall through the time filter.
			return false
		}
		if !f.Since.IsZero() && ts.Before(f.Since) {
			return false
		}
		if !f.Until.IsZero() && !ts.Before(f.Until) {
			return false
		}
	}
	return true
}

// generateID is a minimal sortable id (no external dep). Phase 1 swaps
// to UUIDv7 when we pull in google/uuid for the SQLite/Postgres backends.
func generateID(seq int64) string {
	now := time.Now().UTC().UnixNano()
	return formatHex(now) + "-" + formatHex(seq)
}

func formatHex(v int64) string {
	const hex = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = hex[v&0xf]
		v >>= 4
	}
	return string(buf[i:])
}
