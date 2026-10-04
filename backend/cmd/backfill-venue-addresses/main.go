// Command backfill-venue-addresses gives venues with an empty address one, so
// the street-geocode columns can place them off their city centroid. It works
// in two steps, and only the second writes.
//
// Step 1, look up (the default; writes nothing). For each selected venue:
//
//  1. page: the venue's website, its registered ingest source page, then
//     upcoming-show ticket pages (up to four vendors, at most two read; only
//     shows an admin, a trusted-tier user, or the discovery import created).
//     Instagram and other login-walled hosts are never fetched. A schema.org
//     address is read directly; otherwise the AI extraction path reads the
//     page text (ANTHROPIC_API_KEY; without it only schema.org addresses
//     count, and the lookup keys say so). Ticket pages are read only through
//     schema.org data naming the venue and its city. A hit is geocoded with
//     the structured Nominatim search the street-geocode sweep uses.
//  2. name: for a venue still without an address, a Nominatim search for
//     "<name>, <city>, <state>" in the venue's country. A venue-like place or
//     building whose name and city match supplies the address and the point,
//     stored with geocode_precision = name_search.
//
// The report (--report <path>, required; written as <path>.md and
// <path>.json) lists every phase attempted. Rows that need a person's
// judgement are marked REVIEW: a partial name match, a building rather than a
// venue-like place, a name match for a venue whose own pages failed to load,
// and any row for an unverified venue.
//
// Step 2, apply (--confirm --approved <report.json>). In the JSON report, set
// approve_review to true on each REVIEW row you accept, and set would_write to
// false on any hit you refuse. A corrected address may be typed into a page
// row's address (it is written without the old geocode; the sweep geocodes
// it). The apply step makes no lookups. It writes the accepted hits exactly as
// reviewed, records each refused hit and each miss as a miss for its phase (so
// the next lookup moves on, to the name search after a refused page hit), and
// leaves a deleted row undecided. It skips a row whose venue gained an
// address, had its address cleared, or changed its name, city, state,
// website, or pages since the report. A report older than 90 days, or with
// two rows for one venue and phase, is refused.
//
// A non-empty address is never overwritten, and the city-centroid
// latitude/longitude columns are never touched.
//
// Usage:
//
//	go run ./cmd/backfill-venue-addresses --only-upcoming --limit 100 --report /tmp/addr
//	go run ./cmd/backfill-venue-addresses --city Milwaukee --report /tmp/mke
//	go run ./cmd/backfill-venue-addresses --confirm --approved /tmp/addr.json
//	go run ./cmd/backfill-venue-addresses --env .env.stage --report /tmp/addr
//
// The lookup step's Nominatim requests are limited to one per second
// (NOMINATIM_BASE_URL and NOMINATIM_CONTACT apply as in
// geocode-venue-addresses), and the limiter is per process, so the lookup
// should not run against the public endpoint alongside heavy venue-write
// traffic on a live server sharing the budget. Page fetches honour robots.txt
// and space requests to one host by two seconds.
//
// Exit status: 1 when the apply step had a write error, or when every lookup
// the run attempted errored; per-venue lookup errors on third-party pages are
// listed in the report without failing the run.
package main

import (
	"context"
	"encoding/json"
	"errors"
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
		approvedPath string
		limit        int
		onlyUpcoming bool
		city         string
		reportPath   string
		envFile      string
	)
	flag.BoolVar(&confirm, "confirm", false, "Apply a reviewed report (requires --approved); without it the run only looks up")
	flag.StringVar(&approvedPath, "approved", "", "The reviewed JSON report to apply with --confirm")
	flag.IntVar(&limit, "limit", 0, "Lookup: max venues to look up this run (0 = no limit)")
	flag.BoolVar(&onlyUpcoming, "only-upcoming", false, "Lookup: only venues with a show in the next 90 days (they go first either way)")
	flag.StringVar(&city, "city", "", "Lookup: only venues in this city (case-insensitive)")
	flag.StringVar(&reportPath, "report", "", "Lookup: write the report to <path>.md and <path>.json")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if err := validateFlags(confirm, approvedPath, reportPath, set); err != nil {
		log.Fatal(err)
	}

	loadEnv(envFile)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	mode := "LOOKUP (writes nothing)"
	if confirm {
		mode = "APPLY " + approvedPath
	}
	fmt.Printf("=== Venue Address Backfill: %s ===\n", mode)
	// The resolved target, credentials redacted, so a mistargeted --confirm is
	// caught before any write.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	if confirm {
		os.Exit(apply(approvedPath))
	}
	os.Exit(lookup(cfg, catalog.VenueAddressBackfillOptions{Limit: limit, OnlyUpcoming: onlyUpcoming, City: city}, reportPath))
}

// lookupOnlyFlags select or record a lookup run; an apply writes the rows of
// its report and nothing else, so it takes none of them.
var lookupOnlyFlags = []string{"limit", "only-upcoming", "city", "report"}

// validateFlags checks the flag combination before anything connects: an
// apply needs its reviewed report and takes no lookup flags, and a lookup
// must write a report, since only a report can be applied.
func validateFlags(confirm bool, approvedPath, reportPath string, set map[string]bool) error {
	switch {
	case confirm && approvedPath == "":
		return errors.New("--confirm writes only a reviewed report: run the lookup with --report, review the JSON, then pass it with --approved")
	case !confirm && approvedPath != "":
		return errors.New("--approved is applied only with --confirm")
	case confirm:
		for _, name := range lookupOnlyFlags {
			if set[name] {
				return fmt.Errorf("--%s selects a lookup run; --confirm applies every row of the approved report and takes no lookup flags", name)
			}
		}
	case reportPath == "":
		return errors.New("a lookup needs --report <path>: the JSON report is what --confirm --approved applies")
	}
	return nil
}

func lookup(cfg *config.Config, opts catalog.VenueAddressBackfillOptions, reportPath string) int {
	var ai venueaddress.AddressExtractor
	aiEnabled := cfg.Anthropic.APIKey != ""
	if aiEnabled {
		ai = pipeline.NewExtractionService(nil, cfg, nil, nil)
	} else {
		fmt.Println("ANTHROPIC_API_KEY is empty: pages count only when they publish a schema.org address.")
	}
	nominatim := geo.DefaultNominatim()
	run := &catalog.VenueAddressBackfill{
		DB:        db.GetDB(),
		Pages:     venueaddress.NewFinder(venueaddress.NewFetcher(), ai),
		Places:    nominatim,
		Geocoder:  nominatim,
		AIEnabled: aiEnabled,
	}
	report, runErr := run.Run(context.Background(), opts)
	if report == nil {
		log.Fatalf("lookup: %v", runErr)
	}
	printSummary(report)
	if reportPath != "" {
		if err := writeReport(report, reportPath); err != nil {
			log.Fatalf("write report: %v", err)
		}
	}
	if runErr != nil {
		log.Printf("lookup stopped early: %v", runErr)
		return 1
	}
	attempted := 0
	for _, t := range report.Phases {
		attempted += t.Attempted
	}
	if attempted > 0 && len(report.Errors) == attempted {
		log.Print("every lookup this run attempted errored")
		return 1
	}
	return 0
}

func apply(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read --approved: %v", err)
	}
	var report catalog.VenueAddressReport
	if err := json.Unmarshal(raw, &report); err != nil {
		log.Fatalf("%s is not a backfill JSON report: %v", path, err)
	}
	run := &catalog.VenueAddressBackfill{DB: db.GetDB()}
	result, err := run.Apply(context.Background(), &report)
	if err != nil {
		log.Fatalf("apply: %v", err)
	}
	for _, row := range result.Rows {
		line := fmt.Sprintf("  [%s] venue %d %q %s", row.Action, row.VenueID, row.Name, row.Phase)
		if row.Address != "" {
			line += ": " + row.Address
		}
		if row.Reason != "" {
			line += " (" + row.Reason + ")"
		}
		fmt.Println(line)
	}
	fmt.Printf("\nWritten: %d   misses recorded: %d   refused (recorded as misses): %d   skipped: %d\n",
		result.Written, result.MissesRecorded, result.Refused, result.Skipped)
	for _, e := range result.Errors {
		fmt.Printf("  [ERROR] %s\n", e)
	}
	if len(result.Errors) > 0 {
		return 1
	}
	return 0
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
	fmt.Printf("Candidates (no address): %d   processed: %d   skipped (address cleared): %d\n", r.Candidates, r.Processed, r.SkippedCleared)
	for _, phase := range []string{"page", "name"} {
		t := r.Phases[phase]
		fmt.Printf("  %-5s attempted=%d hits=%d (%.0f%%) misses=%d errors=%d skipped(recorded miss)=%d no-page=%d\n",
			phase, t.Attempted, t.Hits, 100*t.HitRate(), t.Misses, t.Errors, t.SkippedMemo, t.NoSource)
	}
	fmt.Printf("Would write: %d (REVIEW: %d)\n", r.WouldWrite, r.Review)
	labels := make([]string, 0, len(r.Precision))
	for p := range r.Precision {
		labels = append(labels, p)
	}
	sort.Strings(labels)
	for _, p := range labels {
		fmt.Printf("  precision %-12s %d\n", p+":", r.Precision[p])
	}
	if r.LimitHit {
		fmt.Println("Limit reached: applying the reviewed report records its misses and refusals; the next lookup then moves past them (rows deleted from the report, and lookups that errored, come back).")
	}
	for _, e := range r.Errors {
		fmt.Printf("  [ERROR] %s\n", e)
	}
	fmt.Println("\nNothing was written. Review the JSON report, then run with --confirm --approved <report.json>.")
}

// writeReport writes <path>.md and <path>.json; a .md or .json extension on
// path is dropped first so either spelling names the pair. The files are
// owner-only, including when they replace an existing file: they hold street
// addresses, some for unverified venues the site does not publish.
func writeReport(r *catalog.VenueAddressReport, path string) error {
	base := strings.TrimSuffix(strings.TrimSuffix(path, ".md"), ".json")
	if dir := filepath.Dir(base); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for _, out := range []struct {
		path  string
		write func(*os.File) error
	}{
		{base + ".md", func(f *os.File) error { return r.WriteMarkdown(f) }},
		{base + ".json", func(f *os.File) error { return r.WriteJSON(f) }},
	} {
		f, err := os.OpenFile(out.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if err := f.Chmod(0o600); err != nil {
			_ = f.Close()
			return err
		}
		if err := out.write(f); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
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
