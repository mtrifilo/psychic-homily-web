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
// A live run writes only what a person approved. With --approved <report.json>
// (a dry run's JSON report, after review) it considers only the listed venues
// and writes a row only when a fresh lookup finds the same address in the
// same phase; delete a row, or set its would_write to false, to refuse it.
// Without --approved it writes only rows the report did not mark REVIEW.
//
// Usage:
//
//	go run ./cmd/backfill-venue-addresses                          # dry run (default)
//	go run ./cmd/backfill-venue-addresses --only-upcoming --limit 100 --report /tmp/addr
//	go run ./cmd/backfill-venue-addresses --city Milwaukee
//	go run ./cmd/backfill-venue-addresses --confirm --approved /tmp/addr.json  # apply the reviewed rows
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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
		approvedPath string
		noAI         bool
	)
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry run only)")
	flag.IntVar(&limit, "limit", 0, "Max venues to look up this run (0 = no limit)")
	flag.BoolVar(&onlyUpcoming, "only-upcoming", false, "Only venues with a show in the next 90 days (they go first either way)")
	flag.StringVar(&city, "city", "", "Only venues in this city (case-insensitive)")
	flag.StringVar(&reportPath, "report", "", "Write the report to <path>.md and <path>.json")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.StringVar(&approvedPath, "approved", "", "A reviewed dry-run JSON report; a live run writes only the rows it lists")
	flag.BoolVar(&noAI, "no-ai", false, "Allow a live run without ANTHROPIC_API_KEY (pages count only with a schema.org address)")
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
	switch {
	case aiEnabled:
		ai = pipeline.NewExtractionService(nil, cfg, nil, nil)
	case confirm && !noAI:
		log.Fatal("ANTHROPIC_API_KEY is empty: a live run would record page misses the AI path never saw. Set the key, or pass --no-ai to accept that.")
	default:
		fmt.Println("ANTHROPIC_API_KEY is empty: pages count only when they publish a schema.org address.")
	}

	var approved map[catalog.VenueAddressApproval]bool
	if approvedPath != "" {
		approved, err = loadApprovals(approvedPath)
		if err != nil {
			log.Fatalf("read --approved: %v", err)
		}
		fmt.Printf("Approved report: %s (%d rows)\n", approvedPath, len(approved))
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
		Approved:     approved,
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
		if row.Review {
			detail += " REVIEW"
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
	labels := make([]string, 0, len(r.Precision))
	for p := range r.Precision {
		labels = append(labels, p)
	}
	sort.Strings(labels)
	for _, p := range labels {
		fmt.Printf("  precision %-12s %d\n", p+":", r.Precision[p])
	}
	if r.LimitHit {
		fmt.Println("Limit reached: re-run to continue where this run stopped.")
	}
	for _, e := range r.Errors {
		fmt.Printf("  [ERROR] %s\n", e)
	}
	fmt.Println()
}

// loadApprovals reads the would-write rows of a reviewed JSON report.
func loadApprovals(path string) (map[catalog.VenueAddressApproval]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r catalog.VenueAddressReport
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%s is not a backfill JSON report: %w", path, err)
	}
	return catalog.ApprovalsFromReport(&r), nil
}

// writeReport writes <path>.md and <path>.json; a .md or .json extension on
// path is dropped first so either spelling names the pair. The files are
// owner-only: they hold street addresses, some for unverified venues the site
// does not publish.
func writeReport(r *catalog.VenueAddressReport, path string) error {
	base := strings.TrimSuffix(strings.TrimSuffix(path, ".md"), ".json")
	if dir := filepath.Dir(base); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	md, err := os.OpenFile(base+".md", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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
	js, err := os.OpenFile(base+".json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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
