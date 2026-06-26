// Command backfill-cover-art populates release cover art from the Cover Art
// Archive (via MusicBrainz) and Discogs for catalog releases that have none
// (PSY-1216).
//
// This is the bulk cover-art path that complements backfill-spotify-images
// (PSY-1185): Spotify's client-credentials rate limit is a poor fit for a large
// pass, so the primary source here is purpose-built music metadata. For each
// cover-less release it searches MusicBrainz for the release-group (strict-matched
// on title + artist + year), fetches the front cover from the Cover Art Archive,
// and — when CAA has none — falls back to a Discogs search. It stores only a
// REFERENCE to the externally-hosted image (URL + provider id + linkback), never
// the bytes (PSY-1175 architecture D1/D3).
//
// Usage:
//
//	go run ./cmd/backfill-cover-art                    # dry-run (default): search + report, no writes
//	go run ./cmd/backfill-cover-art --confirm          # apply changes
//	go run ./cmd/backfill-cover-art --limit 50         # cap releases processed
//	go run ./cmd/backfill-cover-art --env .env.x       # explicit env file
//	go run ./cmd/backfill-cover-art --discogs-token T  # override DISCOGS_TOKEN
//
// MusicBrainz + the Cover Art Archive need no credentials. Discogs needs a token
// (DISCOGS_TOKEN, or --discogs-token); without one the run is CAA-only. Idempotent:
// only cover-less releases are considered, so a re-run after a live pass reports
// zero updates — which also makes errored releases safe to retry by re-running.
//
// Rate limits are provider-fixed and self-enforced by each client (MusicBrainz
// ~1 req/s shared process-wide per PSY-1208; Discogs ~60/min; CAA is CDN-served),
// so there is no --rps knob and no penalty death-spiral to guard against.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm      bool
	envFile      string
	limit        int
	discogsToken string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.IntVar(&limit, "limit", 0, "Max releases to process (0 = no limit)")
	flag.StringVar(&discogsToken, "discogs-token", "", "Discogs token (defaults to DISCOGS_TOKEN; empty = CAA-only)")
	flag.Parse()

	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if discogsToken == "" {
		discogsToken = cfg.Discogs.Token
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	discogsState := "CAA-only (no Discogs token)"
	if discogsToken != "" {
		discogsState = "CAA + Discogs"
	}
	fmt.Printf("=== Cover Art Enrichment Backfill (%s, %s) — PSY-1216 ===\n\n", mode, discogsState)

	ctx := context.Background()

	// One shared MusicBrainz client (PSY-1208), adapted to the catalog enricher's
	// interface so the catalog package needn't depend on the pipeline package.
	mbAdapter := mbReleaseAdapter{client: pipeline.NewMusicBrainzClient()}

	caaClient := catalog.NewCoverArtArchiveClient()
	defer caaClient.Close()

	opts := catalog.CoverArtEnrichOptions{DryRun: !confirm, Limit: limit}

	// Pass an UNTYPED nil when there is no token — a typed (*catalog.DiscogsClient)(nil)
	// stored in the interface would be non-nil and panic on first method call.
	var report *catalog.CoverArtEnrichReport
	if discogsToken != "" {
		discogsClient := catalog.NewDiscogsClient(discogsToken)
		defer discogsClient.Close()
		report, err = catalog.BackfillCoverArt(ctx, database, mbAdapter, caaClient, discogsClient, opts)
	} else {
		report, err = catalog.BackfillCoverArt(ctx, database, mbAdapter, caaClient, nil, opts)
	}
	if err != nil {
		printReport(report)
		log.Fatalf("backfill failed: %v", err)
	}

	printReport(report)
}

// mbReleaseAdapter adapts the shared pipeline.MusicBrainzClient to the catalog
// enricher's musicBrainzReleaseSearcher interface, flattening each release-group's
// artist credit into the credited + canonical names the strict matcher checks.
type mbReleaseAdapter struct {
	client *pipeline.MusicBrainzClient
}

func (a mbReleaseAdapter) SearchReleaseGroups(ctx context.Context, artist, title string, limit int) ([]catalog.MBReleaseGroupCandidate, error) {
	raw, err := a.client.SearchReleaseGroups(ctx, artist, title, limit)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.MBReleaseGroupCandidate, 0, len(raw))
	for _, rg := range raw {
		names := make([]string, 0, len(rg.ArtistCredit)*2)
		for _, ac := range rg.ArtistCredit {
			if ac.Name != "" {
				names = append(names, ac.Name)
			}
			if ac.Artist.Name != "" && ac.Artist.Name != ac.Name {
				names = append(names, ac.Artist.Name)
			}
		}
		out = append(out, catalog.MBReleaseGroupCandidate{
			MBID:             rg.ID,
			Title:            rg.Title,
			ArtistNames:      names,
			FirstReleaseDate: rg.FirstReleaseDate,
		})
	}
	return out, nil
}

func printReport(r *catalog.CoverArtEnrichReport) {
	if r == nil {
		return
	}
	fmt.Println("=== Summary ===")
	fmt.Printf("Releases:  scanned=%d matched(caa=%d discogs=%d) updated=%d skipped=%d errors=%d\n",
		r.ReleasesScanned, r.ReleasesMatchedCAA, r.ReleasesMatchedDiscogs,
		r.ReleasesUpdated, r.ReleasesSkipped, r.ReleaseErrors)
	fmt.Println()
	if r.ReleaseErrors > 0 {
		fmt.Printf("%d release%s errored and were left unchanged; re-run (idempotent) to retry them.\n",
			r.ReleaseErrors, plural(r.ReleaseErrors, "", "s"))
	}
	if r.DryRun {
		fmt.Println("DRY RUN — searched providers and reported matches; no DB writes.")
		fmt.Println("Re-run with --confirm to store the matched cover references.")
	} else {
		fmt.Println("LIVE — matched cover references committed.")
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
