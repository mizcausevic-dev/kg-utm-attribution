// Package ingest defines the UTM event shape and the validation that
// runs at the edge of the service. Designed so a form-submit handler,
// a pixel beacon, and a server-side conversion all produce the same
// struct — the storage and aggregation layers don't care which surface
// fired the event.
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Event is one attribution event. The five standard UTM parameters are
// preserved verbatim (lowercase) plus operational metadata. IP and
// User-Agent are hashed on the way in so the store never holds raw PII.
type Event struct {
	// UTM parameters (lowercased; empty if not present in the source URL).
	Source   string `json:"utm_source"`
	Medium   string `json:"utm_medium"`
	Campaign string `json:"utm_campaign"`
	Term     string `json:"utm_term,omitempty"`
	Content  string `json:"utm_content,omitempty"`

	// Operational metadata.
	Surface       string `json:"surface"`        // pixel | form | server
	CorrelationID string `json:"correlation_id"` // ties this event to a downstream conversion
	Timestamp     string `json:"timestamp"`      // RFC 3339
	URL           string `json:"url,omitempty"`  // raw landing URL (kept for debugging only; never displayed in dashboards)
	IPHash        string `json:"ip_hash,omitempty"`
	UAHash        string `json:"user_agent_hash,omitempty"`

	// Consent signal at ingest time. The store rejects events whose
	// consent_status doesn't satisfy the active Decision Card policy —
	// see pkg/consent.
	ConsentStatus string `json:"consent_status"` // granted | denied | unknown
}

// Surface enums.
const (
	SurfacePixel  = "pixel"
	SurfaceForm   = "form"
	SurfaceServer = "server"
)

// ConsentStatus enums. Matches the klaviyo-flow-consent-audit terminology.
const (
	ConsentGranted = "granted"
	ConsentDenied  = "denied"
	ConsentUnknown = "unknown"
)

// ErrInvalid is returned when an event fails edge validation.
var ErrInvalid = errors.New("ingest: event invalid")

// FromURL parses the standard UTM parameters out of a landing URL string
// and produces a partial Event. The caller fills in Surface, ConsentStatus,
// CorrelationID, IPHash, and UAHash before persisting.
//
// Lowercased per Google's UTM convention so analytics aggregation doesn't
// fragment on "Facebook" vs "facebook".
func FromURL(landing string) (Event, error) {
	u, err := url.Parse(landing)
	if err != nil {
		return Event{}, fmt.Errorf("%w: parse %s: %v", ErrInvalid, landing, err)
	}
	q := u.Query()
	return Event{
		Source:   strings.ToLower(strings.TrimSpace(q.Get("utm_source"))),
		Medium:   strings.ToLower(strings.TrimSpace(q.Get("utm_medium"))),
		Campaign: strings.ToLower(strings.TrimSpace(q.Get("utm_campaign"))),
		Term:     strings.ToLower(strings.TrimSpace(q.Get("utm_term"))),
		Content:  strings.ToLower(strings.TrimSpace(q.Get("utm_content"))),
		URL:      landing,
	}, nil
}

// Validate runs the edge invariants. Source + Medium + Campaign are
// required (the GA convention) — Term and Content are optional. Surface
// must be one of the enum values. ConsentStatus must be one of the enum
// values. Timestamp must be RFC 3339 if present.
func (e *Event) Validate() error {
	if e.Source == "" {
		return fmt.Errorf("%w: utm_source required", ErrInvalid)
	}
	if e.Medium == "" {
		return fmt.Errorf("%w: utm_medium required", ErrInvalid)
	}
	if e.Campaign == "" {
		return fmt.Errorf("%w: utm_campaign required", ErrInvalid)
	}
	switch e.Surface {
	case SurfacePixel, SurfaceForm, SurfaceServer:
	default:
		return fmt.Errorf("%w: surface must be pixel|form|server, got %q", ErrInvalid, e.Surface)
	}
	switch e.ConsentStatus {
	case ConsentGranted, ConsentDenied, ConsentUnknown:
	default:
		return fmt.Errorf("%w: consent_status must be granted|denied|unknown, got %q", ErrInvalid, e.ConsentStatus)
	}
	if e.Timestamp != "" {
		if _, err := time.Parse(time.RFC3339, e.Timestamp); err != nil {
			return fmt.Errorf("%w: timestamp must be RFC 3339: %v", ErrInvalid, err)
		}
	}
	return nil
}

// HashPII is a convenience for the caller to hash a source IP or
// User-Agent before stamping the event. Same SHA-256 + hex convention
// the audit-stream + visualizer use everywhere.
func HashPII(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
