# kg-utm-attribution

> **v0.1 draft.** Single-binary UTM event ingest + attribution service. Form submits, pixel beacons, and server-side conversions post the same JSON shape; the binary persists them (consent-gated per the buyer's published policy), emits a hash-chained governance event into the Kinetic Gain Suite audit-stream, and exposes REST aggregation by source / medium / campaign / surface + a top/mid/bottom funnel rollup.

Part of the [Kinetic Gain Protocol Suite](https://suite.kineticgain.com). Second Go binary in the portfolio, GTM-vertical companion to [`kg-token-validator`](https://github.com/mizcausevic-dev/kg-token-validator).

```
   form submit ────┐
                   ├──► POST /ingest ──► consent gate ──► store ──► audit-stream
   pixel beacon ───┤                       │ │              │
                   │                  granted? policy  emit utm_attribution.event_ingested
   server convo ───┘                       │
                                           └─► denied → drop + emit event_dropped_no_consent

                    GET /attribution/by-source         buckets desc-by-count + consent split
                    GET /attribution/by-medium         "
                    GET /attribution/by-campaign       "
                    GET /attribution/by-surface        pixel / form / server breakdown
                    GET /attribution/funnel            { top, mid, bottom, total }
```

## Why it exists

Most B2B SaaS attribution stacks force a trade-off: GA4 + Segment-shaped pipelines that fight back when compliance asks "where did this data come from and what's the deletion path?", OR a custom in-house thing that lasts six months before nobody owns it.

`kg-utm-attribution` is the third path: a tiny Go binary that does the boring-and-correct parts well and stays out of the way.

- **Consent-aware at ingest.** Same `granted | denied | unknown` vocabulary as [`klaviyo-flow-consent-audit`](https://github.com/mizcausevic-dev/klaviyo-flow-consent-audit). The configured policy (`consent-required` or `consent-recorded`) decides whether a `denied` event drops at the door or persists with the status alongside.
- **Hash-chained audit at ingest.** Every event — including every drop — lands on the same Suite audit-stream as Decision Card events, reveal events, attestation events. One verifiable record for the data lineage your compliance team actually has to answer for.
- **No PII in dashboards by default.** IP and User-Agent are hashed before the store sees them. Raw landing URL is kept for debugging only, never displayed.
- **Drop-in shape.** Pixel beacons POST a tiny payload; forms POST a slightly bigger one; server-side conversions can POST either or use `/ingest/from-url` to parse the landing URL on our side. All three paths produce the same row.

## Quick start

```bash
# Build (single binary, no runtime deps)
go build -o kg-utm-attribution ./cmd/kg-utm-attribution

# Run with the lenient default policy (consent-recorded)
./kg-utm-attribution \
  --addr :8080 \
  --consent-policy consent-recorded \
  --audit-stream-url http://audit-stream:8080 \
  --audit-source kg-utm-attribution-prod-us-east

# Ingest a pixel-shape event
curl -X POST :8080/ingest -H "Content-Type: application/json" -d '{
  "utm_source": "google",
  "utm_medium": "cpc",
  "utm_campaign": "q2-launch",
  "surface": "pixel",
  "correlation_id": "abc123",
  "consent_status": "granted",
  "timestamp": "2026-05-29T10:00:00Z"
}'

# Or parse it from a landing URL
curl -X POST :8080/ingest/from-url -H "Content-Type: application/json" -d '{
  "url": "https://acme.example/?utm_source=facebook&utm_medium=cpc&utm_campaign=q2-launch",
  "surface": "server",
  "consent_status": "granted"
}'

# See attribution
curl :8080/attribution/by-source
# { "buckets": [ { "key": "google", "count": 12, "granted": 10, "denied": 1, "unknown": 1 }, ... ] }

curl :8080/attribution/funnel
# { "top": 12, "mid": 4, "bottom": 1, "total": 17 }
```

## What's in the binary

| Package | Purpose |
| --- | --- |
| `pkg/ingest` | Defines the `Event` shape + validation. `FromURL(landing)` parses standard UTM parameters and lowercases per GA convention. `HashPII` is a SHA-256 helper so the caller can stamp ip_hash + user_agent_hash before persistence. |
| `pkg/consent` | The consent gate. Two policies in v0.1: `consent-required` (drop everything except `granted`) and `consent-recorded` (persist all, status travels with the event). Phase 1 adds Decision-Card-aware policy loading. |
| `pkg/store` | `Store` interface + in-memory implementation. Phase 0 ships in-memory only; Phase 1 adds SQLite (via pure-Go `modernc.org/sqlite`) and Postgres behind the same interface. |
| `pkg/attribution` | Aggregation queries: `BySource`, `ByMedium`, `ByCampaign`, `BySurface`, `Funnel`. All computed in Go against the `Store` interface — no SQL-specific tricks — so changing backends doesn't change the math. |
| `pkg/audit` | Hash-chained event emitter matching `audit-stream-py`'s canonical-JSON SHA-256 + `prev_hash` convention. Same code as `kg-token-validator/pkg/audit` — both producers append to the same buyer's audit-stream. |
| `internal/server` | The HTTP mux. Eight endpoints: 2 ingest + 5 aggregation + 1 health. |
| `cmd/kg-utm-attribution` | Binary entry. Four flags. |

## Configuration flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--addr` | `:8080` | Listen address |
| `--consent-policy` | `consent-recorded` | `consent-required` (strict) OR `consent-recorded` (permissive — store the status, the consumer filters) |
| `--audit-stream-url` | (empty) | Suite audit-stream endpoint. Empty → emit to local chain only (events still constructed, chain advances) |
| `--audit-source` | `kg-utm-attribution` | Name this instance in emitted events |

## Endpoints

| Method | Path | Returns |
| --- | --- | --- |
| `POST` | `/ingest` | `{ persisted, event_id, verdict, audit_event_id }` (200) OR `{ persisted: false, verdict, reason }` (202, consent drop) |
| `POST` | `/ingest/from-url` | Parse landing URL → same response as `/ingest` |
| `GET` | `/attribution/by-source` | `{ buckets: [{ key, count, granted, denied, unknown }] }` |
| `GET` | `/attribution/by-medium` | (same shape) |
| `GET` | `/attribution/by-campaign` | (same shape) |
| `GET` | `/attribution/by-surface` | (same shape, key in pixel/form/server) |
| `GET` | `/attribution/funnel` | `{ top, mid, bottom, total }` |
| `GET` | `/healthz` | `{ status, version, policy, time }` |

All aggregation endpoints accept query-string filters:

```
?source=google&medium=cpc&campaign=q2-launch&surface=pixel&consent=granted
?since=2026-01-01T00:00:00Z&until=2026-02-01T00:00:00Z&limit=100
```

## Audit events

| Event kind | When | Payload |
| --- | --- | --- |
| `utm_attribution.event_ingested` | every persisted event | event_id, surface, utm_source, utm_medium, utm_campaign, consent_status |
| `utm_attribution.event_dropped_no_consent` | every event dropped by the consent gate | reason, surface, utm_source, utm_campaign, consent_status |

Both follow the canonical-JSON SHA-256 + `prev_hash` chain convention used by every other Suite producer, so they verify cleanly via the visualizer's Audit Stream tab and the [`audit_chain_verify`](https://github.com/mizcausevic-dev/mcp-kinetic-gain) MCP tool.

## Composes with

| Repo | Role |
| --- | --- |
| [`klaviyo-flow-consent-audit`](https://github.com/mizcausevic-dev/klaviyo-flow-consent-audit) | Same `granted/denied/unknown` consent vocabulary; both surfaces feed the buyer's one consent ledger |
| [`audit-stream-py`](https://github.com/mizcausevic-dev/audit-stream-py) | Where every ingest/drop event lands |
| [`kg-token-validator`](https://github.com/mizcausevic-dev/kg-token-validator) | Sibling Go binary; same canonical-JSON SHA-256 audit emitter, different policy axis |
| [`ai-procurement-decision-spec`](https://github.com/mizcausevic-dev/ai-procurement-decision-spec) | Phase 1: the Decision Card declares `consent-policy` per buyer; this binary loads it instead of accepting a CLI flag |

## Phase 1 roadmap

- **SQLite backend** behind the same `Store` interface (pure-Go via `modernc.org/sqlite`, no CGO)
- **Postgres backend** for buyers with existing warehouses
- **Decision Card-aware policy loading** so the consent policy comes from the buyer's signed Decision Card instead of a CLI flag
- **Time-series rollups** (`/attribution/by-day`, `/attribution/by-week`)
- **Retention enforcement** — wire the buyer's Decision Card `retention_envelope[]` to drop events past their TTL with a signed deletion proof
- **Phase 0 → v1.0-prod hardening** pass per the standing squad discipline

## Compliance posture

GTM attribution scaffolding with a consent-aware ingest gate and tamper-evident audit. Supports a buyer's CCPA / GDPR / CASL consent-handling program — does not by itself establish compliance with any of them. Per the standing public-language guardrail: *readiness · evidence · posture · controls · scaffolding* — never "GDPR-compliant" without an external attestation.

## License

MIT.
