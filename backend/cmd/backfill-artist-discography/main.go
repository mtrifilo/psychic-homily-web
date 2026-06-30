// Command backfill-artist-discography imports each artist's PRIMARY discography
// (albums + EPs) from MusicBrainz (PSY-1282). For every artist with a stored MBID
// (PSY-1249) it browses their release-GROUPS by MBID — identity-verified, so no
// homonym risk — creates one release row per release-group (deduped on the
// release-group MBID, the PSY-1281 keystone), maps the primary type + first-release
// year, and fetches cover art directly from the Cover Art Archive by that MBID.
//
// Singles + secondary types (compilation/live/remix/dj-mix/…) are deliberately
// skipped: releases are the highest flood-risk enrichment, so we import the curated
// core only (PSY-1252 decision).
//
// Usage:
//
//	go run ./cmd/backfill-artist-discography                  # dry-run (default)
//	go run ./cmd/backfill-artist-discography --confirm        # apply changes
//	go run ./cmd/backfill-artist-discography --verbose        # per-release-group detail
//	go run ./cmd/backfill-artist-discography --limit 50       # cap artists scanned
//	go run ./cmd/backfill-artist-discography --env .env.stage # target a specific env
//
// Dry-run writes nothing and prints the planned imports. Its "create" count is an
// UPPER BOUND: it detects an existing release only by release-group MBID, not the
// importer's artist-anchored title-match fill, so a live run may dedup some rows the
// dry-run labelled "create". Idempotent: a second --confirm run dedups everything
// (each release-group already exists). MusicBrainz browse is ~1 req/s, so a full run
// over the catalog is long.
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
	"psychic-homily-backend/internal/services/discography"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm bool
	verbose bool
	limit   int
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-release-group detail")
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
	fmt.Printf("=== Artist Discography Backfill (%s) — PSY-1282 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the wrong
	// --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	fmt.Printf("Source: MusicBrainz browse-by-MBID (primary types: album + EP) + Cover Art Archive\n\n")

	report, err := discography.BackfillArtistDiscography(
		database,
		pipeline.NewMusicBrainzClient(),
		catalog.NewCoverArtArchiveClient(),
		discography.Options{
			DryRun: !confirm,
			Limit:  limit,
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

// redactDBHost extracts host[:port]/dbname from a database URL, dropping any embedded
// credentials, so the target can be logged without leaking secrets.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}

func printReport(r *discography.Report) {
	if verbose || !confirm {
		fmt.Println("--- Planned imports ---")
		if len(r.Plans) == 0 {
			fmt.Println("  (none)")
		}
		for _, p := range r.Plans {
			year := "----"
			if p.Year != nil {
				year = fmt.Sprintf("%d", *p.Year)
			}
			fmt.Printf("  [%s] artist %d %q <- %s %q (%s, %s)  rg=%s\n",
				p.Action, p.ArtistID, p.ArtistName, p.Type, p.Title, p.Type, year, p.RGMBID)
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
	fmt.Printf("Artists scanned (have MBID):            %d\n", r.ArtistsScanned)
	fmt.Printf("  artists with no primary releases:    %d\n", r.ArtistsNoReleases)
	fmt.Printf("Release-groups seen (album+EP):         %d\n", r.ReleaseGroupsSeen)
	fmt.Printf("  releases created:                    %d\n", r.Created)
	fmt.Printf("  release-groups already present:      %d\n", r.Deduped)
	fmt.Printf("  cover art set (CAA):                 %d\n", r.CoverArtSet)
	fmt.Printf("Errors:                                %d\n", len(r.Errors))
	fmt.Println()

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
		fmt.Println("(note: 'create' is an upper bound — a live run may dedup some via artist-anchored title match.)")
	} else {
		fmt.Println("LIVE — changes committed.")
	}

	// Exit non-zero if a live run hit errors so CI/cron wrappers can alert.
	if confirm && len(r.Errors) > 0 {
		os.Exit(1)
	}
}
