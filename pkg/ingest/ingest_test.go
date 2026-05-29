package ingest

import (
	"errors"
	"testing"
)

func TestFromURL_StandardUTM(t *testing.T) {
	e, err := FromURL("https://example.com/?utm_source=Google&utm_medium=CPC&utm_campaign=Q2-Launch&utm_term=foo&utm_content=banner-a")
	if err != nil {
		t.Fatal(err)
	}
	// All lowercased.
	if e.Source != "google" {
		t.Errorf("source: want google, got %q", e.Source)
	}
	if e.Medium != "cpc" {
		t.Errorf("medium: want cpc, got %q", e.Medium)
	}
	if e.Campaign != "q2-launch" {
		t.Errorf("campaign: want q2-launch, got %q", e.Campaign)
	}
	if e.Term != "foo" {
		t.Errorf("term: want foo, got %q", e.Term)
	}
	if e.Content != "banner-a" {
		t.Errorf("content: want banner-a, got %q", e.Content)
	}
}

func TestValidate_RequiresSourceMediumCampaign(t *testing.T) {
	cases := []struct {
		name string
		e    Event
	}{
		{"no source", Event{Medium: "cpc", Campaign: "x", Surface: SurfacePixel, ConsentStatus: ConsentGranted}},
		{"no medium", Event{Source: "google", Campaign: "x", Surface: SurfacePixel, ConsentStatus: ConsentGranted}},
		{"no campaign", Event{Source: "google", Medium: "cpc", Surface: SurfacePixel, ConsentStatus: ConsentGranted}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.e.Validate(); err == nil || !errors.Is(err, ErrInvalid) {
				t.Errorf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestValidate_BadSurface(t *testing.T) {
	e := Event{Source: "g", Medium: "cpc", Campaign: "x", Surface: "magic", ConsentStatus: ConsentGranted}
	if err := e.Validate(); err == nil {
		t.Error("expected error for bad surface")
	}
}

func TestValidate_BadConsentStatus(t *testing.T) {
	e := Event{Source: "g", Medium: "cpc", Campaign: "x", Surface: SurfacePixel, ConsentStatus: "maybe"}
	if err := e.Validate(); err == nil {
		t.Error("expected error for bad consent status")
	}
}

func TestHashPII_StableHex(t *testing.T) {
	h1 := HashPII("203.0.113.42")
	h2 := HashPII("203.0.113.42")
	if h1 != h2 {
		t.Error("HashPII not deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("want 64-char hex, got %d", len(h1))
	}
	if HashPII("") != "" {
		t.Error("empty input should hash to empty string")
	}
}
