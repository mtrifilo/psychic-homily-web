// Command backfill-venue-timezones geocodes existing venues (latitude,
// longitude, IANA timezone) from their city/state/country and re-anchors show
// event_date instants that were stored under a wrong assumed timezone before
// the venue-timezone epic (PSY-984). This is PR3 / the backfill step (PSY-987).
//
// Usage:
//
//	go run ./cmd/backfill-venue-timezones                    # dry-run (default)
//	go run ./cmd/backfill-venue-timezones --confirm          # apply changes
//	go run ./cmd/backfill-venue-timezones --verbose          # per-row detail
//	go run ./cmd/backfill-venue-timezones --env .env.stage   # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. The
// re-anchor pass is conservative — it only rewrites shows it can confidently
// recognize as mis-zoned date-only shows and lists anything ambiguous for
// manual review (see services/catalog.reanchorEventDate). Re-anchoring rewrites
// both shows.event_date and the denormalized show_artists.event_date inside a
// per-show transaction. The command is idempotent: a second run reports zero
// changes.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/geo"
)

var (
	confirm bool
	verbose bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-venue / per-show detail")
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
	fmt.Printf("=== Venue Timezone Backfill (%s) — PSY-987 ===\n\n", mode)

	report, err := catalog.BackfillVenueTimezones(database, geo.Default(), catalog.BackfillOptions{
		DryRun:  !confirm,
		Verbose: verbose,
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

func printReport(r *catalog.BackfillReport) {
	fmt.Println("--- Venue geocoding ---")
	for _, c := range r.VenueChanges {
		switch c.Action {
		case "set", "updated":
			fmt.Printf("  [%s] venue %d %q (%s, %s): %s -> %s\n",
				c.Action, c.VenueID, c.Name, c.City, c.State, tzStr(c.OldTz), tzStr(c.NewTz))
		case "miss":
			fmt.Printf("  [miss] venue %d %q (%s, %s): no geocode match (left %s)\n",
				c.VenueID, c.Name, c.City, c.State, tzStr(c.OldTz))
		case "unchanged":
			fmt.Printf("  [unchanged] venue %d %q (%s, %s): %s\n",
				c.VenueID, c.Name, c.City, c.State, tzStr(c.NewTz))
		}
	}

	fmt.Println("\n--- Show re-anchoring ---")
	for _, c := range r.ShowChanges {
		switch c.Action {
		case "reanchored":
			fmt.Printf("  [reanchor] show %d %q (venue %d): %s (%s) -> %s (%s)\n",
				c.ShowID, c.Title, c.VenueID,
				c.OldInstant.Format("2006-01-02T15:04:05Z"), c.AssumedTz,
				c.NewInstant.Format("2006-01-02T15:04:05Z"), c.GeocodedTz)
		case "ambiguous":
			fmt.Printf("  [skip:ambiguous] show %d %q (venue %d): %s — not a recognizable date-only show in %s or %s; left unchanged\n",
				c.ShowID, c.Title, c.VenueID,
				c.OldInstant.Format("2006-01-02T15:04:05Z"), c.GeocodedTz, c.AssumedTz)
		case "no-venue-tz":
			fmt.Printf("  [skip:no-venue-tz] show %d %q (venue %d): venue has no resolved timezone\n",
				c.ShowID, c.Title, c.VenueID)
		}
	}

	if len(r.Errors) > 0 {
		fmt.Println("\n--- Errors ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Venues scanned:    %d\n", r.VenuesScanned)
	fmt.Printf("  tz set:          %d\n", r.VenuesSet)
	fmt.Printf("  tz updated:      %d\n", r.VenuesUpdated)
	fmt.Printf("  unchanged:       %d\n", r.VenuesUnchanged)
	fmt.Printf("  no geocode hit:  %d\n", r.VenuesMissed)
	fmt.Printf("Shows scanned:     %d\n", r.ShowsScanned)
	fmt.Printf("  re-anchored:     %d\n", r.ShowsReanchored)
	fmt.Printf("  already correct: %d\n", r.ShowsAlreadyOK)
	fmt.Printf("  ambiguous skip:  %d\n", r.ShowsAmbiguous)
	fmt.Printf("  no venue tz:     %d\n", r.ShowsNoVenueTz)
	fmt.Printf("Errors:            %d\n", len(r.Errors))
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

func tzStr(s *string) string {
	if s == nil || *s == "" {
		return "<none>"
	}
	return *s
}
