// Command geocode-venue-addresses resolves street-level coordinates for
// venues.address via Nominatim/OpenStreetMap and stores them in the
// street_latitude/street_longitude/geocode_precision/geocoded_address columns
// (PSY-1536). The existing latitude/longitude city-centroid columns are never
// touched.
//
// Usage:
//
//	go run ./cmd/geocode-venue-addresses                   # dry-run (default)
//	go run ./cmd/geocode-venue-addresses --confirm         # apply changes
//	go run ./cmd/geocode-venue-addresses --limit 25        # cap network lookups
//	go run ./cmd/geocode-venue-addresses --env .env.stage  # target a specific env
//
// NOTE: a dry run performs the SAME Nominatim lookups as a live run (so the
// printed per-venue results and precision labels are real) — it just writes
// nothing. Lookups are rate-limited to 1 request/second per the OSM usage
// policy, so a large first run takes roughly one second per venue that has an
// un-geocoded address. Venues whose stored geocoded_address already matches
// their current address are skipped without a network call — hits AND
// recorded misses — making a clean second run instant. NOMINATIM_BASE_URL
// overrides the endpoint (self-hosted instance or a local stub);
// NOMINATIM_CONTACT sets the User-Agent contact channel.
//
// The 1 req/s limiter is per-process: do NOT run this against the public
// endpoint while the live server is taking venue-write traffic (inline
// geocoding shares the same budget from a separate process). Run it
// off-hours, or point NOMINATIM_BASE_URL at a self-hosted instance.
//
// Steady-state reconciliation is handled by the in-server daily sweep
// (catalog.StreetGeocodeSweep, PSY-1544), which shares the server's limiter;
// this CLI remains for initial catalog-wide backfills, large imports, and
// one-off dry-run inspection.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/geo"
)

var (
	confirm bool
	limit   int
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.IntVar(&limit, "limit", 0, "Max venues to geocode over the network this run (0 = no limit)")
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
	fmt.Printf("=== Venue Street-Address Geocoding (%s) — PSY-1536 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	report, err := catalog.BackfillVenueStreetGeocodes(context.Background(), database, geo.DefaultNominatim(), catalog.StreetGeocodeOptions{
		DryRun: !confirm,
		Limit:  limit,
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

func printReport(r *catalog.StreetGeocodeReport) {
	fmt.Println("--- Per-venue results ---")
	for _, c := range r.Changes {
		switch c.Action {
		case catalog.StreetGeocodeSet, catalog.StreetGeocodeUpdated:
			fmt.Printf("  [%s] venue %d %q (%s, %s): %f,%f precision=%s key=%q\n",
				c.Action, c.VenueID, c.Name, c.City, c.State,
				c.Latitude, c.Longitude, c.Precision, c.Key)
		case catalog.StreetGeocodeMiss:
			fmt.Printf("  [miss] venue %d %q (%s, %s): no Nominatim match for %q — miss recorded\n",
				c.VenueID, c.Name, c.City, c.State, c.Key)
		case catalog.StreetGeocodeMissCleared:
			fmt.Printf("  [miss] venue %d %q (%s, %s): no Nominatim match for %q — STALE COORDS CLEARED, miss recorded\n",
				c.VenueID, c.Name, c.City, c.State, c.Key)
		case catalog.StreetGeocodeCleared:
			fmt.Printf("  [cleared] venue %d %q (%s, %s): address removed — street geocode cleared\n",
				c.VenueID, c.Name, c.City, c.State)
		case catalog.StreetGeocodeError:
			fmt.Printf("  [ERROR] venue %d %q (%s, %s): %s\n",
				c.VenueID, c.Name, c.City, c.State, c.Err)
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Venues scanned:      %d\n", r.Scanned)
	fmt.Printf("  geocoded (new):    %d\n", r.Set)
	fmt.Printf("  re-geocoded:       %d\n", r.Updated)
	fmt.Printf("  unchanged (skip):  %d\n", r.Unchanged)
	fmt.Printf("  no match:          %d\n", r.Missed)
	fmt.Printf("  cleared (stale):   %d\n", r.Cleared)
	fmt.Printf("  no address:        %d\n", r.NoAddress)
	fmt.Printf("Errors:              %d\n", len(r.Errors))
	if r.LimitHit {
		fmt.Println("Limit reached — re-run to continue where this run stopped.")
	}

	if len(r.PrecisionCounts) > 0 {
		fmt.Println("\nPrecision distribution:")
		labels := make([]string, 0, len(r.PrecisionCounts))
		for p := range r.PrecisionCounts {
			labels = append(labels, p)
		}
		sort.Strings(labels)
		for _, p := range labels {
			fmt.Printf("  %-13s %d\n", p+":", r.PrecisionCounts[p])
		}
	}

	if len(r.Errors) > 0 {
		fmt.Println("\n--- Errors ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
	}

	fmt.Println()
	if confirm {
		fmt.Println("LIVE — changes committed.")
	} else {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	}

	// Exit non-zero whenever lookups errored — dry runs included — so
	// wrappers/automation can't mistake a fully-failed dry run for a clean one.
	if len(r.Errors) > 0 {
		os.Exit(1)
	}
}
