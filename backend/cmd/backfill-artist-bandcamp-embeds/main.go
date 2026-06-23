// Command backfill-artist-bandcamp-embeds derives artists.bandcamp_embed_url
// from each artist's catalogued release Bandcamp links (PSY-1188). Many artists
// have only a Bandcamp PROFILE ROOT in social.bandcamp and an empty embed, so
// the artist page falls back to a plain text link even when a release links to a
// perfectly embeddable album/track URL. This backfill fills the embed from those
// release links and stamps the provenance "release_derived" so the PSY-1189
// keep-fresh hook can later refresh/clean up the auto-derived ones without
// touching human-curated embeds.
//
// FILL-WHEN-EMPTY: only artists with a NULL bandcamp_embed_url are touched; a
// non-null value is never overwritten.
//
// Usage:
//
//	go run ./cmd/backfill-artist-bandcamp-embeds                  # dry-run (default)
//	go run ./cmd/backfill-artist-bandcamp-embeds --confirm        # apply changes
//	go run ./cmd/backfill-artist-bandcamp-embeds --verbose        # per-artist detail
//	go run ./cmd/backfill-artist-bandcamp-embeds --env .env.stage # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. The
// command is idempotent: a second --confirm run reports zero fills (now-non-null
// rows are excluded by the IS NULL gate).
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
	verbose bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-artist detail")
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
	fmt.Printf("=== Artist Bandcamp Embed Backfill (%s) — PSY-1188 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	report, err := catalog.BackfillArtistBandcampEmbeds(database, catalog.BandcampEmbedBackfillOptions{
		DryRun: !confirm,
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

func printReport(r *catalog.BandcampEmbedBackfillReport) {
	if verbose || !confirm {
		fmt.Println("--- Planned fills ---")
		if len(r.Fills) == 0 {
			fmt.Println("  (none)")
		}
		for _, f := range r.Fills {
			fmt.Printf("  [fill] artist %d %q -> %s\n", f.ArtistID, f.Name, f.EmbedURL)
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
	fmt.Printf("Artists scanned (embed IS NULL): %d\n", r.ArtistsScanned)
	fmt.Printf("  filled (release-derived):      %d\n", r.Filled)
	fmt.Printf("  skipped (no release link):     %d\n", r.SkippedNoLink)
	fmt.Printf("Left (embed already set):        %d\n", r.Left)
	fmt.Printf("Errors:                          %d\n", len(r.Errors))
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
