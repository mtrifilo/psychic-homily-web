// Command backfill-venue-addresses gives venues with an empty address one, in
// two phases, so the street-geocode columns can place them off their city
// centroid:
//
//  1. page: the venue's website, its registered ingest source page, then up to
//     two upcoming-show ticket pages (distinct hosts). Instagram and other
//     login-walled hosts are never fetched. A schema.org address on the page
//     is read directly; otherwise the AI extraction path reads the page text
//     (needs ANTHROPIC_API_KEY; without it only schema.org addresses count).
//     A hit is geocoded with the same structured Nominatim search the
//     street-geocode sweep uses.
//  2. name: for a venue still without an address, a Nominatim search for
//     "<name>, <city>, <state>" in the venue's country. A venue-like place or
//     building whose name and city match supplies the address and the street
//     point, stored with geocode_precision = name_search.
//
// A non-empty address is never overwritten, and the city-centroid
// latitude/longitude columns are never touched. Misses are recorded per phase
// so a re-run skips them until the venue's inputs change.
//
// Usage:
//
//	go run ./cmd/backfill-venue-addresses                          # dry run (default)
//	go run ./cmd/backfill-venue-addresses --only-upcoming --limit 100 --report /tmp/addr
//	go run ./cmd/backfill-venue-addresses --city Milwaukee
//	go run ./cmd/backfill-venue-addresses --confirm                # apply
//	go run ./cmd/backfill-venue-addresses --env .env.stage         # target a specific env
//
// A dry run makes the SAME page fetches, AI calls, and Nominatim requests as a
// live run; it writes nothing. Nominatim requests are limited to one per
// second (NOMINATIM_BASE_URL and NOMINATIM_CONTACT apply as in
// geocode-venue-addresses), and the limiter is per process: run this off-hours
// against the public endpoint, never alongside a live server's venue-write
// traffic. Page fetches honour robots.txt and space requests to one host by
// two seconds.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/venueaddress"
)

func main() {
	var (
		confirm      bool
		limit        int
		onlyUpcoming bool
		city         string
		reportPath   string
		envFile      string
	)
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry run only)")
	flag.IntVar(&limit, "limit", 0, "Max venues to look up this run (0 = no limit)")
	flag.BoolVar(&onlyUpcoming, "only-upcoming", false, "Only venues with a show in the next 90 days (they go first either way)")
	flag.StringVar(&city, "city", "", "Only venues in this city (case-insensitive)")
	flag.StringVar(&reportPath, "report", "", "Write the report to <path>.md and <path>.json")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	loadEnv(envFile)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Venue Address Backfill (%s) ===\n", mode)
	// The resolved target, credentials redacted, so a mistargeted --confirm is
	// caught before any write.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	var ai venueaddress.AddressExtractor
	aiEnabled := cfg.Anthropic.APIKey != ""
	if aiEnabled {
		ai = pipeline.NewExtractionService(nil, cfg, nil, nil)
	} else {
		fmt.Println("ANTHROPIC_API_KEY is empty: pages count only when they publish a schema.org address.")
	}
	fmt.Println()

	nominatim := geo.DefaultNominatim()
	run := &catalog.VenueAddressBackfill{
		DB:        db.GetDB(),
		Pages:     venueaddress.NewFinder(venueaddress.NewFetcher(), ai),
		Places:    nominatim,
		Geocoder:  nominatim,
		AIEnabled: aiEnabled,
	}
	report, runErr := run.Run(context.Background(), catalog.VenueAddressBackfillOptions{
		DryRun:       !confirm,
		Limit:        limit,
		OnlyUpcoming: onlyUpcoming,
		City:         city,
	})
	if report == nil {
		log.Fatalf("backfill: %v", runErr)
	}

	printSummary(report)
	if reportPath != "" {
		if err := writeReport(report, reportPath); err != nil {
			log.Fatalf("write report: %v", err)
		}
	}
	if runErr != nil {
		log.Fatalf("backfill stopped early: %v", runErr)
	}
	if confirm {
		fmt.Println("LIVE: changes committed.")
	} else {
		fmt.Println("DRY RUN: no DB writes. Re-run with --confirm to apply.")
	}
	// Exit non-zero whenever a lookup errored, dry runs included, so a wrapper
	// cannot mistake a failed run for a clean one.
	if len(report.Errors) > 0 {
		os.Exit(1)
	}
}

func printSummary(r *catalog.VenueAddressReport) {
	fmt.Println("--- Per-venue results ---")
	for _, row := range r.Rows {
		detail := row.Address
		if row.Precision != "" {
			detail += " precision=" + row.Precision
		}
		fmt.Printf("  [%s/%s] venue %d %q (%s, %s): %s  source=%s\n",
			row.Phase, row.Outcome, row.VenueID, row.Name, row.City, row.State, detail, row.Source)
	}
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Candidates (no address): %d   processed: %d\n", r.Candidates, r.Processed)
	for _, phase := range []string{"page", "name"} {
		t := r.Phases[phase]
		fmt.Printf("  %-5s attempted=%d hits=%d (%.0f%%) misses=%d errors=%d skipped(recorded miss)=%d no-page=%d\n",
			phase, t.Attempted, t.Hits, 100*t.HitRate(), t.Misses, t.Errors, t.SkippedMemo, t.NoSource)
	}
	fmt.Printf("Would write: %d   written: %d\n", r.WouldWrite, r.Written)
	for p, n := range r.Precision {
		fmt.Printf("  precision %-12s %d\n", p+":", n)
	}
	if r.LimitHit {
		fmt.Println("Limit reached: re-run to continue where this run stopped.")
	}
	for _, e := range r.Errors {
		fmt.Printf("  [ERROR] %s\n", e)
	}
	fmt.Println()
}

// writeReport writes <path>.md and <path>.json; a .md or .json extension on
// path is dropped first so either spelling names the pair.
func writeReport(r *catalog.VenueAddressReport, path string) error {
	base := strings.TrimSuffix(strings.TrimSuffix(path, ".md"), ".json")
	if dir := filepath.Dir(base); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	md, err := os.Create(base + ".md")
	if err != nil {
		return err
	}
	if err := r.WriteMarkdown(md); err != nil {
		_ = md.Close()
		return err
	}
	if err := md.Close(); err != nil {
		return err
	}
	js, err := os.Create(base + ".json")
	if err != nil {
		return err
	}
	if err := r.WriteJSON(js); err != nil {
		_ = js.Close()
		return err
	}
	if err := js.Close(); err != nil {
		return err
	}
	fmt.Printf("Report written to %s.md and %s.json\n", base, base)
	return nil
}

func loadEnv(envFile string) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			log.Fatalf("load env file %s: %v", envFile, err)
		}
		log.Printf("loaded env from %s", envFile)
		return
	}
	for _, ef := range []string{".env.development", ".env"} {
		if err := godotenv.Load(ef); err == nil {
			log.Printf("loaded env from %s", ef)
			return
		}
	}
	log.Println("no .env loaded; using process environment")
}

// redactDBHost extracts host[:port]/dbname from a database URL, dropping any
// embedded credentials, so the target can be logged without leaking secrets.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}
