// Package server exposes ingest + attribution as a small HTTP service.
//
// Endpoints:
//
//	POST /ingest                            consent-gated event ingest
//	POST /ingest/from-url                   parse landing URL, then ingest
//	GET  /attribution/by-source             aggregation
//	GET  /attribution/by-medium             aggregation
//	GET  /attribution/by-campaign           aggregation
//	GET  /attribution/by-surface            aggregation
//	GET  /attribution/funnel                top/mid/bottom rollup
//	GET  /healthz                           liveness + build info
//
// Filter query params are accepted on the attribution endpoints:
//
//	?source=foo&medium=bar&campaign=launch&surface=pixel&consent=granted
//	?since=2026-01-01T00:00:00Z&until=2026-02-01T00:00:00Z&limit=100
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/attribution"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/audit"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/consent"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/store"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Server wires the dependencies and routes.
type Server struct {
	Store  store.Store
	Audit  *audit.Emitter
	Policy consent.Policy
	Aggreg *attribution.Aggregator
}

// New constructs a Server.
func New(s store.Store, em *audit.Emitter, policy consent.Policy) *Server {
	return &Server{
		Store:  s,
		Audit:  em,
		Policy: policy,
		Aggreg: attribution.New(s),
	}
}

// Handler returns the HTTP mux for this server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", s.ingestHandler)
	mux.HandleFunc("POST /ingest/from-url", s.ingestFromURLHandler)
	mux.HandleFunc("GET /attribution/by-source", s.aggregateHandler(s.Aggreg.BySource))
	mux.HandleFunc("GET /attribution/by-medium", s.aggregateHandler(s.Aggreg.ByMedium))
	mux.HandleFunc("GET /attribution/by-campaign", s.aggregateHandler(s.Aggreg.ByCampaign))
	mux.HandleFunc("GET /attribution/by-surface", s.aggregateHandler(s.Aggreg.BySurface))
	mux.HandleFunc("GET /attribution/funnel", s.funnelHandler)
	mux.HandleFunc("GET /healthz", s.healthzHandler)
	return logRequests(mux)
}

// ─── ingest ────────────────────────────────────────────────────────────────

func (s *Server) ingestHandler(w http.ResponseWriter, r *http.Request) {
	var e ingest.Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	s.persist(r.Context(), w, e)
}

func (s *Server) ingestFromURLHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL           string `json:"url"`
		Surface       string `json:"surface"`
		CorrelationID string `json:"correlation_id"`
		ConsentStatus string `json:"consent_status"`
		IPHash        string `json:"ip_hash"`
		UAHash        string `json:"user_agent_hash"`
		Timestamp     string `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	e, err := ingest.FromURL(body.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e.Surface = body.Surface
	e.CorrelationID = body.CorrelationID
	e.ConsentStatus = body.ConsentStatus
	e.IPHash = body.IPHash
	e.UAHash = body.UAHash
	e.Timestamp = body.Timestamp
	s.persist(r.Context(), w, e)
}

func (s *Server) persist(ctx context.Context, w http.ResponseWriter, e ingest.Event) {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if err := e.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	verdict, reason, err := consent.Gate(s.Policy, e)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if verdict == consent.VerdictDropNoConsent {
		_, _ = s.Audit.Emit(ctx, "utm_attribution.event_dropped_no_consent", map[string]any{
			"reason":         reason,
			"surface":        e.Surface,
			"utm_source":     e.Source,
			"utm_campaign":   e.Campaign,
			"consent_status": e.ConsentStatus,
		})
		writeJSON(w, http.StatusAccepted, map[string]any{
			"persisted": false,
			"verdict":   string(verdict),
			"reason":    reason,
		})
		return
	}

	id, err := s.Store.Insert(ctx, e)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ev, _ := s.Audit.Emit(ctx, "utm_attribution.event_ingested", map[string]any{
		"event_id":       id,
		"surface":        e.Surface,
		"utm_source":     e.Source,
		"utm_medium":     e.Medium,
		"utm_campaign":   e.Campaign,
		"consent_status": e.ConsentStatus,
	})
	resp := map[string]any{
		"persisted": true,
		"event_id":  id,
		"verdict":   string(verdict),
	}
	if ev != nil {
		resp["audit_event_id"] = ev.EventID
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── aggregations ──────────────────────────────────────────────────────────

func (s *Server) aggregateHandler(fn func(context.Context, store.Filter) ([]attribution.Bucket, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := parseFilter(r)
		buckets, err := fn(r.Context(), f)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets})
	}
}

func (s *Server) funnelHandler(w http.ResponseWriter, r *http.Request) {
	f := parseFilter(r)
	rollup, err := s.Aggreg.Funnel(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rollup)
}

// ─── support ───────────────────────────────────────────────────────────────

func (s *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": Version,
		"policy":  string(s.Policy),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func parseFilter(r *http.Request) store.Filter {
	q := r.URL.Query()
	f := store.Filter{
		Source:        q.Get("source"),
		Medium:        q.Get("medium"),
		Campaign:      q.Get("campaign"),
		Surface:       q.Get("surface"),
		ConsentStatus: q.Get("consent"),
	}
	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = t
		}
	}
	if s := q.Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Until = t
		}
	}
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			f.Limit = n
		}
	}
	return f
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		log.Printf("%s %s → %d (%s)", r.Method, r.URL.Path, ww.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
