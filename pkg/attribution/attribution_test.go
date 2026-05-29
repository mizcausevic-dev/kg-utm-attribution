package attribution

import (
	"context"
	"testing"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/store"
)

func TestBySource(t *testing.T) {
	s := storeWithFixtures(t)
	a := New(s)
	out, err := a.BySource(context.Background(), store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	// 4 events: google×2, linkedin×2. Sort desc-by-count then asc-by-key →
	// "google" + "linkedin" each with 2.
	if len(out) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(out))
	}
	if out[0].Key != "google" {
		t.Errorf("first bucket: want google, got %s", out[0].Key)
	}
	if out[0].Count != 2 {
		t.Errorf("google count: want 2, got %d", out[0].Count)
	}
}

func TestByCampaign_ConsentSplit(t *testing.T) {
	s := storeWithFixtures(t)
	a := New(s)
	out, _ := a.ByCampaign(context.Background(), store.Filter{Campaign: "q2-launch"})
	if len(out) != 1 {
		t.Fatalf("want 1 bucket (q2-launch), got %d", len(out))
	}
	b := out[0]
	if b.Granted != 1 || b.Denied != 1 {
		t.Errorf("q2-launch consent split: want granted=1 denied=1, got granted=%d denied=%d",
			b.Granted, b.Denied)
	}
}

func TestFunnel(t *testing.T) {
	s := storeWithFixtures(t)
	a := New(s)
	r, _ := a.Funnel(context.Background(), store.Filter{})
	// fixtures: 2 pixel + 1 form + 1 server.
	if r.Top != 2 || r.Mid != 1 || r.Bottom != 1 || r.Total != 4 {
		t.Errorf("funnel: want 2/1/1/4, got %d/%d/%d/%d", r.Top, r.Mid, r.Bottom, r.Total)
	}
}

func storeWithFixtures(t *testing.T) *store.Memory {
	t.Helper()
	s := store.NewMemory()
	ctx := context.Background()
	for _, e := range []ingest.Event{
		{Source: "google", Medium: "cpc", Campaign: "q1-launch", Surface: "pixel", ConsentStatus: "granted", Timestamp: "2026-01-15T10:00:00Z"},
		{Source: "google", Medium: "organic", Campaign: "q1-launch", Surface: "form", ConsentStatus: "granted", Timestamp: "2026-01-20T10:00:00Z"},
		{Source: "linkedin", Medium: "social", Campaign: "q2-launch", Surface: "pixel", ConsentStatus: "denied", Timestamp: "2026-02-15T10:00:00Z"},
		{Source: "linkedin", Medium: "social", Campaign: "q2-launch", Surface: "server", ConsentStatus: "granted", Timestamp: "2026-02-20T10:00:00Z"},
	} {
		_, _ = s.Insert(ctx, e)
	}
	return s
}
