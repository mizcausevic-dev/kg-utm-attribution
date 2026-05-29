# Changelog — kg-utm-attribution

## v0.1 — 2026-05-29 (draft)

Second Go binary in the portfolio. UTM event ingest + attribution service with consent-gated persistence + hash-chained audit-stream emission. ~9.3 MB distroless container.

What landed:

- `pkg/ingest` — `Event` struct + `FromURL` parser (standard UTM params, lowercased per GA) + edge `Validate()` + `HashPII` helper. 5 unit tests
- `pkg/consent` — `Policy` enum (`consent-required` / `consent-recorded`) + `Gate(policy, event)` returning verdict + audit-friendly reason. 3 unit tests
- `pkg/store` — `Store` interface (Insert / Query / Count) + in-memory implementation. Filter by source/medium/campaign/surface/consent/time-range/limit. Phase 1 swaps in SQLite (modernc.org/sqlite, no CGO) + Postgres. 3 unit tests
- `pkg/attribution` — `Aggregator` over `Store`. `BySource`, `ByMedium`, `ByCampaign`, `BySurface` all return `Bucket{Key, Count, Granted, Denied, Unknown}` sorted desc-by-count. `Funnel` returns `{top, mid, bottom, total}` with pixel/form/server mapped to top/mid/bottom. 3 unit tests
- `pkg/audit` — hash-chained event emitter (verbatim from kg-token-validator — same canonical-JSON SHA-256 + prev_hash convention). 3 unit tests
- `internal/server` — `net/http` mux: 2 ingest + 5 aggregation + 1 health endpoint. Filter query params on aggregations. 2 integration tests against `httptest`
- `cmd/kg-utm-attribution` — 4-flag binary. CGO_ENABLED=0 builds.
- `examples/sample-events.jsonl` — 5 events covering all 3 surfaces + all 3 consent statuses
- `Dockerfile` — multi-stage distroless build, nonroot, ~10 MB final image
- `Makefile` + `.github/workflows/ci.yml` (matrix Go 1.22 + 1.23) + `LICENSE` (MIT) + `.gitignore`

Verification:
- `go vet ./...` — clean
- `gofmt -l .` — clean
- `go test ./...` — 6 packages pass (ingest, consent, store, attribution, audit, internal/server)
- `go build` — single 9.3 MB binary

Phase 1 roadmap:
- SQLite + Postgres backends behind the same `Store` interface
- Decision Card-aware policy loading (consent policy comes from the buyer's signed Decision Card instead of a CLI flag)
- Time-series rollups (`/attribution/by-day`, `/by-week`)
- Retention enforcement wired to the Decision Card's `retention_envelope[]` (drop events past their TTL with a signed deletion proof)
- v1.0-prod hardening pass
