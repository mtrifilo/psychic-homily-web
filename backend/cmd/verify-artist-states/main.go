// Command verify-artist-states re-checks artists that ALREADY have a state and
// corrects a wrong one — but ONLY when MusicBrainz identity-confirms a different
// state for the same artist (PSY-1255 cleanup).
//
// Background: the blocked PSY-1244 pass wrote the highest-population US namesake's
// state for every city-only artist. That is right for the dominant city
// (Austin→TX) but wrong for a smaller one (Pasadena→TX, not CA), and the two are
// indistinguishable offline. This pass re-derives the state through the same
// identity-confirmed path as the fill backfill (a MusicBrainz candidate is
// trusted only when its url-rels share the artist's own Spotify/Bandcamp link)
// and OVERWRITES the stored state only on a confirmed disagreement. It never
// NULLs and never guesses, so it cannot destroy a correct state:
//
//   - geocoder unambiguously confirms the stored state → left as-is (no MB call)
//   - MusicBrainz identity-confirms the SAME state      → confirmed, left as-is
//   - MusicBrainz identity-confirms a DIFFERENT state   → corrected (overwritten)
//   - can't confirm (no link / not in MB / non-US)      → left as-is
//
// Usage:
//
//	go run ./cmd/verify-artist-states                    # dry-run (default)
//	go run ./cmd/verify-artist-states --confirm          # apply corrections
//	go run ./cmd/verify-artist-states --limit 50         # cap artists scanned
//	go run ./cmd/verify-artist-states --env .env.stage   # target a specific env
//
// MusicBrainz is rate-limited to ~1 req/s and the pass calls it once per
// non-geocoder-confirmed, US, linked artist (plus a url-rels and maybe an
// area-rels lookup), so a full run takes minutes. A sustained MB outage trips a
// circuit breaker that disables MB for the rest of the run.
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
	confirm bool
	limit   int
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply corrections (default: dry-run only)")
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
	fmt.Printf("=== Artist State Verify + Correct (%s) — PSY-1255 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	report, err := enrich.VerifyArtistStates(database, geo.Default(), pipeline.NewMusicBrainzClient(), enrich.VerifyOptions{
		DryRun: !confirm,
		Limit:  limit,
	})
	if err != nil {
		log.Fatalf("verify: %v", err)
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

func printReport(r *enrich.VerifyReport) {
	fmt.Println("--- Corrections (MusicBrainz identity-confirmed a different state) ---")
	if len(r.Corrections) == 0 {
		fmt.Println("  (none)")
	}
	for _, c := range r.Corrections {
		fmt.Printf("  [fix] artist %d %q  %q  %s -> %s\n", c.ArtistID, c.Name, c.City, c.OldState, c.NewState)
	}
	fmt.Println()

	if len(r.Errors) > 0 {
		fmt.Println("--- Errors ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
		fmt.Println()
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Artists scanned (city + state set):        %d\n", r.ArtistsScanned)
	fmt.Printf("  geocoder-confirmed (skipped, no MB):     %d\n", r.DefiniteOK)
	fmt.Printf("  MusicBrainz confirmed same state:        %d\n", r.Confirmed)
	fmt.Printf("  CORRECTED (different state written):     %d\n", r.Corrected)
	fmt.Printf("  unverified (no link / not in MB / non-US): %d\n", r.Unverified)
	fmt.Printf("Errors:                                    %d\n", len(r.Errors))
	fmt.Println()

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	} else {
		fmt.Println("LIVE — corrections committed.")
	}

	// Exit non-zero if a live run hit errors so CI/cron wrappers can alert.
	if confirm && len(r.Errors) > 0 {
		os.Exit(1)
	}
}
