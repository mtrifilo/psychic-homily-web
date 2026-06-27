// Command backfill-artist-state derives a US state for artists that have a city
// but no state, so a city-only MusicBrainz artist becomes matchable by the
// scenes local-artist filter (PSY-1233), which keys on city AND state.
//
// State comes from the SOURCE, never a population guess (PSY-1255, absorbing the
// blocked PSY-1244). Two layers:
//
//	1. Offline geocoder: fills the state ONLY for a city name that maps to one US
//	   state (Chicago -> IL). A multi-state namesake (Pasadena CA/TX) is NOT
//	   guessed — that was the bug that wrote the wrong state.
//	2. MusicBrainz: for the ambiguous residual, re-search the artist and trust a
//	   name-matched candidate only when it names the SAME city, then read the
//	   city's parent Subdivision (an extra rate-limited area-rels lookup). Skip
//	   --musicbrainz with --geocoder-only for a fast, network-free first pass.
//
// FILL-WHEN-EMPTY: only artists with a NULL/blank state are touched. Anything the
// two layers can't confirm is left NULL (a review bucket) rather than guessed.
//
// Usage:
//
//	go run ./cmd/backfill-artist-state                    # dry-run (default)
//	go run ./cmd/backfill-artist-state --confirm          # apply changes
//	go run ./cmd/backfill-artist-state --geocoder-only    # offline only, no MB
//	go run ./cmd/backfill-artist-state --verbose          # per-artist detail
//	go run ./cmd/backfill-artist-state --limit 50         # cap artists scanned
//	go run ./cmd/backfill-artist-state --env .env.stage   # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. The
// command is idempotent: a second --confirm run reports zero fills (now-set rows
// drop out of the "city set, state empty" gate).
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
	"psychic-homily-backend/internal/services/enrich"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm      bool
	geocoderOnly bool
	verbose      bool
	limit        int
	envFile      string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&geocoderOnly, "geocoder-only", false, "Skip MusicBrainz; leave ambiguous cities unresolved")
	flag.BoolVar(&verbose, "verbose", false, "Print per-artist detail")
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
	fmt.Printf("=== Artist State Derivation (%s) — PSY-1255 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	source := "unambiguous geocoder + MusicBrainz area-rels (ambiguous)"
	if geocoderOnly {
		source = "unambiguous geocoder only (ambiguous cities left unresolved)"
	}
	fmt.Printf("Source: %s\n\n", source)

	// Hold the resolver as the interface type so --geocoder-only passes a true nil
	// interface (not a typed-nil *MusicBrainzClient, which would defeat the
	// backfill's `mb != nil` gate and panic on the first method call).
	var mb enrich.MBStateResolver
	if !geocoderOnly {
		mb = pipeline.NewMusicBrainzClient()
	}

	report, err := enrich.BackfillArtistStates(database, geo.Default(), mb, enrich.StateOptions{
		DryRun:       !confirm,
		Limit:        limit,
		GeocoderOnly: geocoderOnly,
	})
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

func printReport(r *enrich.StateReport) {
	if verbose || !confirm {
		fmt.Println("--- Planned fills ---")
		if len(r.Fills) == 0 {
			fmt.Println("  (none)")
		}
		for _, f := range r.Fills {
			fmt.Printf("  [fill] artist %d %q  %q -> state %q (via %s)\n",
				f.ArtistID, f.Name, f.City, f.State, f.Source)
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
	fmt.Printf("Artists scanned (city set, state empty): %d\n", r.ArtistsScanned)
	fmt.Printf("  state filled from geocoder (unambig):  %d\n", r.FilledGeo)
	fmt.Printf("  state filled from MusicBrainz (ambig): %d\n", r.FilledMusicBrainz)
	fmt.Printf("  ambiguous, MusicBrainz unconfirmed:    %d\n", r.AmbiguousUnresolved)
	fmt.Printf("  skipped (non-US / no US state):        %d\n", r.Skipped)
	fmt.Printf("Errors:                                  %d\n", len(r.Errors))
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
