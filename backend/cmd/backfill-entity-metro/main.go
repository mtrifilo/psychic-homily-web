// Command backfill-entity-metro reconciles the denormalized `metro` (US Census
// CBSA code) column on artists and venues (PSY-1255 step B).
//
// metro is DERIVED from (city, state, country) via the offline geocoder, so it
// must equal that derivation for the Atlas scene rollup to be correct. The create
// write paths set it, but enrichment fills and state corrections change an
// entity's location WITHOUT touching metro, so it drifts. This command recomputes
// metro for every row (offline — no network) and writes only the ones that
// differ. Run it AFTER a location or state backfill. It is idempotent: a clean
// second run reports zero changes.
//
// Usage:
//
//	go run ./cmd/backfill-entity-metro                  # dry-run (default)
//	go run ./cmd/backfill-entity-metro --confirm        # apply
//	go run ./cmd/backfill-entity-metro --env .env.stage # target a specific env
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
	"psychic-homily-backend/internal/services/geo"
)

var (
	confirm bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
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
	g := geo.Default()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Entity Metro Reconcile (%s) — PSY-1255 step B ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	artists, err := catalog.ReconcileArtistMetros(database, g, !confirm)
	if err != nil {
		log.Fatalf("artists: %v", err)
	}
	printReport("artists", artists)

	venues, err := catalog.ReconcileVenueMetros(database, g, !confirm)
	if err != nil {
		log.Fatalf("venues: %v", err)
	}
	printReport("venues", venues)

	festivals, err := catalog.ReconcileFestivalMetros(database, g, !confirm)
	if err != nil {
		log.Fatalf("festivals: %v", err)
	}
	printReport("festivals", festivals)

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	} else {
		fmt.Println("LIVE — changes committed.")
	}
	if confirm && (len(artists.Errors) > 0 || len(venues.Errors) > 0 || len(festivals.Errors) > 0) {
		os.Exit(1)
	}
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

func printReport(label string, r *catalog.MetroReport) {
	fmt.Printf("--- %s ---\n", label)
	fmt.Printf("  scanned:                 %d\n", r.Scanned)
	fmt.Printf("  set (was NULL):          %d\n", r.Set)
	fmt.Printf("  changed (stale → fixed): %d\n", r.Changed)
	fmt.Printf("  cleared (no longer res): %d\n", r.Cleared)
	fmt.Printf("  unchanged:               %d\n", r.Unchanged)
	if len(r.Errors) > 0 {
		fmt.Printf("  errors:                  %d\n", len(r.Errors))
		for _, e := range r.Errors {
			fmt.Printf("    [ERROR] %s\n", e)
		}
	}
	fmt.Println()
}
