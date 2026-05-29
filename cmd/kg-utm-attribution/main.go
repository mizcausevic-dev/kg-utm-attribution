// kg-utm-attribution is a single-binary UTM event ingest + attribution
// service. Pixel beacons, form submissions, and server-side conversions
// all post the same shape; the binary persists them (consent-gated),
// emits a hash-chained governance event into the Suite audit-stream, and
// exposes REST aggregation by source/medium/campaign/surface + a funnel
// rollup.
//
// Usage:
//
//	kg-utm-attribution \
//	  --addr :8080 \
//	  --consent-policy consent-recorded \
//	  --audit-stream-url http://audit-stream:8080 \
//	  --audit-source kg-utm-attribution-prod-us-east
//
// Phase 0 ships an in-memory store. Phase 1 adds SQLite + Postgres
// backends behind the same store.Store interface.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mizcausevic-dev/kg-utm-attribution/internal/server"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/audit"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/consent"
	"github.com/mizcausevic-dev/kg-utm-attribution/pkg/store"
)

func main() {
	var (
		addr           = flag.String("addr", ":8080", "address to listen on")
		policyFlag     = flag.String("consent-policy", "consent-recorded", "consent-required | consent-recorded")
		auditStreamURL = flag.String("audit-stream-url", "", "Suite audit-stream endpoint (empty: degrade to local chain)")
		auditSource    = flag.String("audit-source", "kg-utm-attribution", "name this instance in emitted events")
	)
	flag.Parse()

	policy := consent.Policy(*policyFlag)
	if policy != consent.PolicyConsentRequired && policy != consent.PolicyConsentRecorded {
		log.Printf("error: --consent-policy must be 'consent-required' or 'consent-recorded', got %q", *policyFlag)
		flag.Usage()
		os.Exit(2)
	}

	em := audit.NewEmitter(*auditStreamURL, *auditSource)
	st := store.NewMemory()
	srv := server.New(st, em, policy)

	log.Printf("kg-utm-attribution listening on %s", *addr)
	log.Printf("  consent-policy=%s", policy)
	if *auditStreamURL == "" {
		log.Printf("  audit-stream=<none — emitting to local chain only>")
	} else {
		log.Printf("  audit-stream=%s (source=%s)", *auditStreamURL, *auditSource)
	}
	log.Printf("  store=memory (Phase 0 default)")

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
