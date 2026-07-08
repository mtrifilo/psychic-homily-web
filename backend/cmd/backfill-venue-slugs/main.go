// Command backfill-venue-slugs rewrites venues whose stored slug does not match
// the canonical utils.GenerateVenueSlug(name, city, state) output. Historical
// seed data (PSY-1385) left a few venues with corrupted slugs — the first
// character of each word dropped, the state suffix missing, or the slug empty
// (e.g. "alley-ar-hoenix" for Valley Bar in Phoenix, AZ). Every current
// create/update path already uses GenerateVenueSlug, so this is a one-shot
// cleanup and is idempotent: a second run reports zero changes.
//
// Usage:
//
//	go run ./cmd/backfill-venue-slugs                 # dry-run (default)
//	go run ./cmd/backfill-venue-slugs --confirm       # apply changes
//	go run ./cmd/backfill-venue-slugs --env .env.stage # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. There
// is no slug-redirect mechanism in the system and the corrupted slugs have no
// legitimate external references, so old slugs simply 404 after the rewrite;
// internal links regenerate from venue.slug.
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

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Venue Slug Backfill (%s) — PSY-1385 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	report, err := catalog.BackfillVenueSlugs(database, catalog.VenueSlugBackfillOptions{DryRun: !confirm})
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

func printReport(r *catalog.VenueSlugBackfillReport) {
	fmt.Println("--- Slug changes ---")
	if len(r.Changes) == 0 {
		fmt.Println("  (none — all venue slugs already canonical)")
	}
	for _, c := range r.Changes {
		status := "would-update"
		if c.Applied {
			status = "updated"
		}
		old := c.OldSlug
		if old == "" {
			old = "<empty>"
		}
		fmt.Printf("  [%s] venue %d %q (%s, %s): %s -> %s\n",
			status, c.VenueID, c.Name, c.City, c.State, old, c.NewSlug)
	}

	if len(r.Errors) > 0 {
		fmt.Println("\n--- Errors ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Venues scanned:  %d\n", r.Scanned)
	fmt.Printf("  changed:       %d\n", r.Changed)
	fmt.Printf("  unchanged:     %d\n", r.Unchanged)
	fmt.Printf("  errors:        %d\n", len(r.Errors))
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
