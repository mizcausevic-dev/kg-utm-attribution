// Package consent gates ingestion on the buyer's consent policy.
//
// Composes with klaviyo-flow-consent-audit's same vocabulary
// (granted | denied | unknown). The decision-card-aware Phase 1 will
// load the buyer's Decision Card and apply policy:
//
//   - "consent_required" mode: drop events with consent_status != granted
//   - "consent_recorded" mode: persist all events, store the status
//
// Phase 0 ships the two named modes hardcoded; the policy enum is the
// extension point.
package consent

import (
	"fmt"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
)

// Policy controls how the gate treats incoming events by consent status.
type Policy string

const (
	// PolicyConsentRequired drops events whose consent_status != granted.
	// Use when the buyer's Decision Card declares opt-in is required
	// (GDPR member states, California CCPA-strict tenants, etc.).
	PolicyConsentRequired Policy = "consent-required"

	// PolicyConsentRecorded persists every event regardless of status,
	// but the status field travels with the event so downstream consumers
	// can filter. Use when the buyer's Decision Card declares legitimate
	// interest or opt-out (most B2B SaaS in the US).
	PolicyConsentRecorded Policy = "consent-recorded"
)

// Verdict is the consent gate's answer.
type Verdict string

const (
	VerdictAccept        Verdict = "accept"
	VerdictDropNoConsent Verdict = "drop:no-consent"
)

// ErrUnknownPolicy is returned when the gate is configured with a value
// not in the enum.
var ErrUnknownPolicy = fmt.Errorf("consent: unknown policy")

// Gate evaluates an event against the configured policy. Returns a
// verdict and a human-readable reason suitable for audit-stream
// emission.
func Gate(policy Policy, e ingest.Event) (Verdict, string, error) {
	switch policy {
	case PolicyConsentRequired:
		if e.ConsentStatus == ingest.ConsentGranted {
			return VerdictAccept, "consent granted", nil
		}
		return VerdictDropNoConsent,
			fmt.Sprintf("policy consent-required: consent_status=%s", e.ConsentStatus),
			nil
	case PolicyConsentRecorded:
		return VerdictAccept,
			fmt.Sprintf("policy consent-recorded: consent_status=%s", e.ConsentStatus),
			nil
	default:
		return "", "", fmt.Errorf("%w: %q", ErrUnknownPolicy, policy)
	}
}
