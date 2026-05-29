// Package attribution turns raw events in the store into aggregated
// dashboards: by-source, by-medium, by-campaign, plus a funnel-stage
// rollup.
//
// All aggregations are computed in Go against the Store interface — no
// SQL-specific tricks — so swapping the storage backend doesn't change
// the math.
package attribution

import (
	"context"
	"sort"
	"strings"

	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/store"
)

// Bucket is one row of an aggregation.
type Bucket struct {
	Key     string `json:"key"`     // utm_source / utm_medium / utm_campaign value (or "<none>")
	Count   int    `json:"count"`   // events in this bucket
	Granted int    `json:"granted"` // events with consent_status=granted
	Denied  int    `json:"denied"`  // events with consent_status=denied
	Unknown int    `json:"unknown"` // events with consent_status=unknown
}

// Aggregator runs the queries.
type Aggregator struct {
	Store store.Store
}

// New constructs an Aggregator.
func New(s store.Store) *Aggregator { return &Aggregator{Store: s} }

// BySource groups events by utm_source.
func (a *Aggregator) BySource(ctx context.Context, f store.Filter) ([]Bucket, error) {
	return a.groupBy(ctx, f, func(e *eventView) string { return e.Source })
}

// ByMedium groups events by utm_medium.
func (a *Aggregator) ByMedium(ctx context.Context, f store.Filter) ([]Bucket, error) {
	return a.groupBy(ctx, f, func(e *eventView) string { return e.Medium })
}

// ByCampaign groups events by utm_campaign.
func (a *Aggregator) ByCampaign(ctx context.Context, f store.Filter) ([]Bucket, error) {
	return a.groupBy(ctx, f, func(e *eventView) string { return e.Campaign })
}

// BySurface groups events by ingest surface (pixel / form / server).
func (a *Aggregator) BySurface(ctx context.Context, f store.Filter) ([]Bucket, error) {
	return a.groupBy(ctx, f, func(e *eventView) string { return e.Surface })
}

// eventView is the narrow read shape the aggregator needs. Keeps the
// dependency surface on pkg/ingest small.
type eventView struct {
	Source        string
	Medium        string
	Campaign      string
	Surface       string
	ConsentStatus string
}

func (a *Aggregator) groupBy(ctx context.Context, f store.Filter, key func(*eventView) string) ([]Bucket, error) {
	events, err := a.Store.Query(ctx, f)
	if err != nil {
		return nil, err
	}
	buckets := map[string]*Bucket{}
	for i := range events {
		ev := events[i]
		v := &eventView{
			Source:        ev.Source,
			Medium:        ev.Medium,
			Campaign:      ev.Campaign,
			Surface:       ev.Surface,
			ConsentStatus: ev.ConsentStatus,
		}
		k := key(v)
		if k == "" {
			k = "<none>"
		}
		b, ok := buckets[k]
		if !ok {
			b = &Bucket{Key: k}
			buckets[k] = b
		}
		b.Count++
		switch v.ConsentStatus {
		case "granted":
			b.Granted++
		case "denied":
			b.Denied++
		default:
			b.Unknown++
		}
	}
	out := make([]Bucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	// Sort desc by Count, then asc by Key for stable output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.Compare(out[i].Key, out[j].Key) < 0
	})
	return out, nil
}

// Funnel returns three counts (top of funnel, mid, conversion) for a
// given filter. The funnel-stage mapping is conventional:
//
//   - pixel  → top of funnel (impression / page view)
//   - form   → mid funnel (lead capture)
//   - server → bottom of funnel (server-side conversion)
//
// Buyers who have a different funnel can reorder in their dashboard.
type FunnelRollup struct {
	Top    int `json:"top"`    // pixel events
	Mid    int `json:"mid"`    // form events
	Bottom int `json:"bottom"` // server events
	Total  int `json:"total"`
}

func (a *Aggregator) Funnel(ctx context.Context, f store.Filter) (FunnelRollup, error) {
	events, err := a.Store.Query(ctx, f)
	if err != nil {
		return FunnelRollup{}, err
	}
	var r FunnelRollup
	for _, e := range events {
		switch e.Surface {
		case "pixel":
			r.Top++
		case "form":
			r.Mid++
		case "server":
			r.Bottom++
		}
	}
	r.Total = r.Top + r.Mid + r.Bottom
	return r, nil
}
