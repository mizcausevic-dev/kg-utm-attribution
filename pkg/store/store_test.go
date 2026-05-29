package store

import (
	"context"
	"testing"
	"time"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
)

func TestMemory_InsertQueryCount(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	for _, e := range fixtures() {
		if _, err := m.Insert(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	all, err := m.Query(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Errorf("Query{}: want 4, got %d", len(all))
	}

	google, _ := m.Query(ctx, Filter{Source: "google"})
	if len(google) != 2 {
		t.Errorf("Query{source=google}: want 2, got %d", len(google))
	}

	n, _ := m.Count(ctx, Filter{Source: "google", Surface: "pixel"})
	if n != 1 {
		t.Errorf("Count{source=google,surface=pixel}: want 1, got %d", n)
	}
}

func TestMemory_FilterByTime(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for _, e := range fixtures() {
		_, _ = m.Insert(ctx, e)
	}

	since, _ := time.Parse(time.RFC3339, "2026-02-01T00:00:00Z")
	out, _ := m.Query(ctx, Filter{Since: since})
	if len(out) != 2 {
		t.Errorf("since 2026-02: want 2 of 4, got %d", len(out))
	}
}

func TestMemory_QueryReturnsSorted(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// Insert out of timestamp order.
	for _, e := range []ingest.Event{
		{Source: "g", Medium: "m", Campaign: "c", Surface: "pixel", ConsentStatus: "granted", Timestamp: "2026-03-01T00:00:00Z"},
		{Source: "g", Medium: "m", Campaign: "c", Surface: "pixel", ConsentStatus: "granted", Timestamp: "2026-01-01T00:00:00Z"},
		{Source: "g", Medium: "m", Campaign: "c", Surface: "pixel", ConsentStatus: "granted", Timestamp: "2026-02-01T00:00:00Z"},
	} {
		_, _ = m.Insert(ctx, e)
	}
	out, _ := m.Query(ctx, Filter{})
	want := []string{"2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z"}
	for i, w := range want {
		if out[i].Timestamp != w {
			t.Errorf("idx %d: want %s, got %s", i, w, out[i].Timestamp)
		}
	}
}

func fixtures() []ingest.Event {
	return []ingest.Event{
		{Source: "google", Medium: "cpc", Campaign: "q1-launch", Surface: "pixel", ConsentStatus: "granted", Timestamp: "2026-01-15T10:00:00Z"},
		{Source: "google", Medium: "organic", Campaign: "q1-launch", Surface: "form", ConsentStatus: "granted", Timestamp: "2026-01-20T10:00:00Z"},
		{Source: "linkedin", Medium: "social", Campaign: "q2-launch", Surface: "pixel", ConsentStatus: "denied", Timestamp: "2026-02-15T10:00:00Z"},
		{Source: "linkedin", Medium: "social", Campaign: "q2-launch", Surface: "server", ConsentStatus: "granted", Timestamp: "2026-02-20T10:00:00Z"},
	}
}
