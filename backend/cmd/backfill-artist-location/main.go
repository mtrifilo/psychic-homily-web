// Command backfill-artist-location enriches artist city/state/country from data
// we already fetch but discard (PSY-1234): the MusicBrainz artist search
// response's area/begin-area/country (primary — curated origin) and the band's
// self-reported location on its Bandcamp profile page (fallback).
//
// FILL-WHEN-EMPTY: only an artist's NULL/blank location fields are touched; a
// set field is never overwritten. Each fill stamps provenance
// (data_source / source_confidence / last_verified_at), and an existing
// data_source from another enrichment is preserved.
//
// Usage:
//
//	go run ./cmd/backfill-artist-location                    # dry-run (default)
//	go run ./cmd/backfill-artist-location --confirm          # apply changes
//	go run ./cmd/backfill-artist-location --verbose          # per-artist detail
//	go run ./cmd/backfill-artist-location --limit 50         # cap artists scanned
//	go run ./cmd/backfill-artist-location --bandcamp-only    # skip MusicBrainz
//	go run ./cmd/backfill-artist-location --env .env.stage   # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. The
// command is idempotent: a second --confirm run reports zero fills (now-set rows
// drop out of the "needs location" gate). MusicBrainz is rate-limited to ~1
// req/s, so a full fallback run over the catalog is minutes-long.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/enrich"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm      bool
	verbose      bool
	bandcampOnly bool
	limit        int
	envFile      string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-artist detail")
	flag.BoolVar(&bandcampOnly, "bandcamp-only", false, "Skip the MusicBrainz fallback")
	flag.IntVar(&limit, "limit", 0, "Max artists to scan (0 = all)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Artist Location Backfill (%s) — PSY-1234 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	source := "MusicBrainz primary + Bandcamp fallback"
	if bandcampOnly {
		source = "Bandcamp only"
	}
	fmt.Printf("Source: %s\n\n", source)

	report, err := enrich.BackfillArtistLocations(
		database,
		catalog.NewBandcampProfileResolver(),
		pipeline.NewMusicBrainzClient(),
		enrich.Options{
			DryRun:       !confirm,
			Limit:        limit,
			BandcampOnly: bandcampOnly,
		},
	)
	if err != nil {
		log.Fatalf("backfill: %v", err)
	}

	printReport(report)
}

func loadEnv() {
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

func printReport(r *enrich.Report) {
	if verbose || !confirm {
		fmt.Println("--- Planned fills ---")
		if len(r.Fills) == 0 {
			fmt.Println("  (none)")
		}
		for _, f := range r.Fills {
			fmt.Printf("  [fill] artist %d %q <- %s %v  {city:%q state:%q country:%q}\n",
				f.ArtistID, f.Name, f.Source, f.Fields, f.Location.City, f.Location.State, f.Location.Country)
		}
		fmt.Println()
	}

	if len(r.Conflicts) > 0 {
		fmt.Println("--- Conflicts (sources disagree on country — skipped for review) ---")
		for _, c := range r.Conflicts {
			fmt.Printf("  [conflict] artist %d %q  MB{%s/%s/%s} vs Bandcamp{%s/%s/%s}\n",
				c.ArtistID, c.Name,
				c.MB.City, c.MB.State, c.MB.Country,
				c.Bandcamp.City, c.Bandcamp.State, c.Bandcamp.Country)
		}
		fmt.Println()
	}

	if len(r.Errors) > 0 {
		fmt.Println("--- Errors ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
		fmt.Println()
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Artists scanned (missing city):         %d\n", r.ArtistsScanned)
	fmt.Printf("  filled from Bandcamp:                %d\n", r.FilledBandcamp)
	fmt.Printf("  filled from MusicBrainz:             %d\n", r.FilledMusicBrainz)
	fmt.Printf("  resolved but nothing empty to fill:  %d\n", r.ResolvedNoFill)
	fmt.Printf("  MusicBrainz MBIDs stamped:           %d\n", r.StampedMBID)
	fmt.Printf("  conflicts (skipped for review):      %d\n", len(r.Conflicts))
	fmt.Printf("  missed (no source had a location):   %d\n", r.Missed)
	fmt.Printf("Errors:                                %d\n", len(r.Errors))
	fmt.Println()

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	} else {
		fmt.Println("LIVE — changes committed.")
	}

	// Exit non-zero if a live run hit errors so CI/cron wrappers can alert.
	if confirm && len(r.Errors) > 0 {
		os.Exit(1)
	}
}
