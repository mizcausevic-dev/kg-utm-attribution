package consent

import (
	"errors"
	"testing"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/ingest"
)

func TestGate_ConsentRequired(t *testing.T) {
	cases := []struct {
		status string
		want   Verdict
	}{
		{ingest.ConsentGranted, VerdictAccept},
		{ingest.ConsentDenied, VerdictDropNoConsent},
		{ingest.ConsentUnknown, VerdictDropNoConsent},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			v, _, err := Gate(PolicyConsentRequired, ingest.Event{ConsentStatus: tc.status})
			if err != nil {
				t.Fatal(err)
			}
			if v != tc.want {
				t.Errorf("want %s, got %s", tc.want, v)
			}
		})
	}
}

func TestGate_ConsentRecorded(t *testing.T) {
	// All three statuses accept under "consent-recorded".
	for _, s := range []string{ingest.ConsentGranted, ingest.ConsentDenied, ingest.ConsentUnknown} {
		v, _, err := Gate(PolicyConsentRecorded, ingest.Event{ConsentStatus: s})
		if err != nil {
			t.Fatal(err)
		}
		if v != VerdictAccept {
			t.Errorf("status=%s: want accept, got %s", s, v)
		}
	}
}

func TestGate_UnknownPolicy(t *testing.T) {
	_, _, err := Gate("bogus", ingest.Event{ConsentStatus: ingest.ConsentGranted})
	if !errors.Is(err, ErrUnknownPolicy) {
		t.Errorf("want ErrUnknownPolicy, got %v", err)
	}
}
