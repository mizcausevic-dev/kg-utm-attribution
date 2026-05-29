package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/audit"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/consent"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/store"
)

func TestServer_IngestThenAggregate(t *testing.T) {
	srv := New(store.NewMemory(), audit.NewEmitter("", "test"), consent.PolicyConsentRecorded)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Ingest two events from /ingest (raw JSON event).
	post(t, ts.URL+"/ingest", map[string]any{
		"utm_source":     "google",
		"utm_medium":     "cpc",
		"utm_campaign":   "q1-launch",
		"surface":        "pixel",
		"correlation_id": "abc",
		"consent_status": "granted",
		"timestamp":      "2026-01-15T10:00:00Z",
	})
	post(t, ts.URL+"/ingest", map[string]any{
		"utm_source":     "linkedin",
		"utm_medium":     "social",
		"utm_campaign":   "q2-launch",
		"surface":        "form",
		"correlation_id": "def",
		"consent_status": "denied",
		"timestamp":      "2026-02-15T10:00:00Z",
	})

	// /ingest/from-url path.
	post(t, ts.URL+"/ingest/from-url", map[string]any{
		"url":            "https://x.example/?utm_source=facebook&utm_medium=cpc&utm_campaign=q3-launch",
		"surface":        "server",
		"correlation_id": "ghi",
		"consent_status": "granted",
		"timestamp":      "2026-03-01T10:00:00Z",
	})

	// /attribution/by-source should return 3 buckets, descending.
	resp := getJSON(t, ts.URL+"/attribution/by-source")
	buckets, _ := resp["buckets"].([]any)
	if len(buckets) != 3 {
		t.Fatalf("by-source: want 3 buckets, got %d", len(buckets))
	}

	// /attribution/funnel should report Top=1 Mid=1 Bottom=1 Total=3.
	r := getJSON(t, ts.URL+"/attribution/funnel")
	if r["total"].(float64) != 3 {
		t.Errorf("funnel total: want 3, got %v", r["total"])
	}
}

func TestServer_ConsentRequired_DropsDenied(t *testing.T) {
	srv := New(store.NewMemory(), audit.NewEmitter("", "test"), consent.PolicyConsentRequired)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/ingest", "application/json", bytes.NewReader(mustMarshal(t, map[string]any{
		"utm_source":     "google",
		"utm_medium":     "cpc",
		"utm_campaign":   "q1",
		"surface":        "pixel",
		"consent_status": "denied",
		"timestamp":      "2026-01-01T00:00:00Z",
	})))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("want 202 Accepted (consent dropped), got %d", resp.StatusCode)
	}

	r := getJSON(t, ts.URL+"/attribution/by-source")
	buckets, _ := r["buckets"].([]any)
	if len(buckets) != 0 {
		t.Errorf("want 0 buckets (event dropped), got %d", len(buckets))
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func post(t *testing.T, url string, body map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader(mustMarshal(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Errorf("POST %s: status %d", url, resp.StatusCode)
	}
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
