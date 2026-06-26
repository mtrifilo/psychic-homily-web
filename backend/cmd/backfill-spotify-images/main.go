// Command backfill-spotify-images populates release cover art and artist photos
// from Spotify for catalog entities that have none (PSY-1185).
//
// It stores only a REFERENCE to the Spotify-hosted image — the image URL plus
// the provider id ("spotify") and a deep linkback — never the image bytes
// (PSY-1175 architecture D1/D3). Covers come from a strict-matched album search
// (exact normalized title + artist, year within one); artist photos come from
// dereferencing the operator-curated Spotify link. Ambiguous results are skipped
// and logged, never blindly stored.
//
// Usage:
//
//	go run ./cmd/backfill-spotify-images                 # dry-run (default): search + report, no writes
//	go run ./cmd/backfill-spotify-images --confirm       # apply changes
//	go run ./cmd/backfill-spotify-images --limit 50      # cap entities per type (covers + artists)
//	go run ./cmd/backfill-spotify-images --rps 2         # tune request rate (default 1/s, conservative)
//	go run ./cmd/backfill-spotify-images --env .env.x    # explicit env file
//
// Requires SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET (client-credentials flow).
// Idempotent: only entities missing an image are considered, so a re-run after a
// live pass reports zero updates — which also makes errored entities safe to retry
// by simply re-running.
//
// Rate limiting: Spotify's limit is an unpublished rolling window. --rps sets a
// conservative steady cadence to avoid tripping it; if a 429 happens anyway, the
// client honors the Retry-After and rides it out. If the throttle won't clear
// (e.g. a leftover penalty from an earlier burst), the run ABORTS with a clear
// message rather than grinding — wait a few minutes and re-run (idempotent).
//
// NOTE: with --limit unset (0), the run loads every image-less release (+ its
// artists) and every image-less artist into memory at once. That is fine at the
// current catalog scale; for a very large catalog, run in batches with --limit N
// (safe + resumable precisely because the run is idempotent).
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

var (
	confirm bool
	envFile string
	limit   int
	rps     float64
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.IntVar(&limit, "limit", 0, "Max entities to process per type (0 = no limit)")
	flag.Float64Var(&rps, "rps", 1.0, "Max Spotify API requests per second (tune against the rolling rate limit)")
	flag.Parse()

	if rps <= 0 {
		log.Fatalf("--rps must be greater than 0 (got %g)", rps)
	}

	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.Spotify.ClientID == "" || cfg.Spotify.ClientSecret == "" {
		log.Fatalf("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set (image enrichment uses the client-credentials flow)")
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}

	// Derive the inter-request interval from --rps (validated > 0 above). Spotify's
	// rate limit is an unpublished rolling window; a conservative cadence avoids the
	// 429 throttle that stalls a large pass.
	rateLimit := time.Duration(float64(time.Second) / rps)
	fmt.Printf("=== Spotify Image Enrichment Backfill (%s, %.2g req/s) — PSY-1185 ===\n\n", mode, rps)

	client := catalog.NewSpotifyClient(cfg.Spotify.ClientID, cfg.Spotify.ClientSecret, rateLimit)
	defer client.Close()

	report, err := catalog.BackfillSpotifyImages(database, client, catalog.SpotifyEnrichOptions{
		DryRun: !confirm,
		Limit:  limit,
	})
	if err != nil {
		// A mid-run error returns a partial report; print what we have, then fail.
		printReport(report)
		log.Fatalf("backfill failed: %v", err)
	}

	printReport(report)
}

func printReport(r *catalog.SpotifyEnrichReport) {
	if r == nil {
		return
	}
	fmt.Println("=== Summary ===")
	fmt.Printf("Releases:  scanned=%d matched=%d updated=%d skipped=%d errors=%d\n",
		r.ReleasesScanned, r.ReleasesMatched, r.ReleasesUpdated, r.ReleasesSkipped, r.ReleaseErrors)
	fmt.Printf("Artists:   scanned=%d matched=%d updated=%d skipped=%d errors=%d\n",
		r.ArtistsScanned, r.ArtistsMatched, r.ArtistsUpdated, r.ArtistsSkipped, r.ArtistErrors)
	fmt.Println()
	if errs := r.ReleaseErrors + r.ArtistErrors; errs > 0 {
		fmt.Printf("%d entit%s errored and were left unchanged; re-run (idempotent) to retry them.\n",
			errs, plural(errs, "y", "ies"))
	}
	if r.DryRun {
		fmt.Println("DRY RUN — searched Spotify and reported matches; no DB writes.")
		fmt.Println("Re-run with --confirm to store the matched image references.")
	} else {
		fmt.Println("LIVE — matched image references committed.")
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
