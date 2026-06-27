// Command backfill-commons-photos populates artist photos from Wikimedia Commons
// for artists that have none (PSY-1232).
//
// This is the Spotify-free, durable artist-photo path (the spike PSY-1228 measured
// ~26% catalog coverage). For each photo-less artist it resolves the MusicBrainz
// artist (ID-anchored on a shared external link when the name is ambiguous) →
// Wikidata P18 (image) → the Commons file, and stores only a REFERENCE to the
// freely-licensed image (URL + provider id + linkback + CC license + author),
// never the bytes (PSY-1175 architecture D1/D3).
//
// Usage:
//
//	go run ./cmd/backfill-commons-photos                # dry-run (default): resolve + report, no writes
//	go run ./cmd/backfill-commons-photos --confirm      # apply changes
//	go run ./cmd/backfill-commons-photos --limit 50     # cap artists processed
//	go run ./cmd/backfill-commons-photos --env .env.x   # explicit env file
//
// No credentials needed (MusicBrainz / Wikidata / Commons are auth-free). Only
// freely-licensed images (CC-*, CC0, public domain) are stored. Idempotent: only
// photo-less artists are considered, so a re-run after a live pass reports zero
// updates — which makes errored artists safe to retry by re-running.
//
// Rates are provider-fixed + self-enforced (MusicBrainz ~1 req/s shared
// process-wide per PSY-1208; Wikidata/Commons are gently spaced), so there is no
// --rps knob.
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
	confirm bool
	envFile string
	limit   int
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.IntVar(&limit, "limit", 0, "Max artists to process (0 = no limit)")
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
	fmt.Printf("=== Commons Artist-Photo Enrichment Backfill (%s) — PSY-1232 ===\n\n", mode)

	ctx := context.Background()

	// One shared MusicBrainz client (PSY-1208), adapted to the catalog enricher's
	// interfaces so the catalog package needn't depend on the pipeline package.
	mb := mbArtistAdapter{client: pipeline.NewMusicBrainzClient()}

	wd := catalog.NewWikidataClient()
	defer wd.Close()
	commons := catalog.NewCommonsClient()
	defer commons.Close()

	report, err := catalog.BackfillCommonsPhotos(ctx, database, mb, wd, commons, catalog.CommonsEnrichOptions{
		DryRun: !confirm,
		Limit:  limit,
	})
	if err != nil {
		printReport(report)
		log.Fatalf("backfill failed: %v", err)
	}

	printReport(report)
}

// mbArtistAdapter adapts the shared pipeline.MusicBrainzClient to the catalog
// enricher's musicBrainzArtistAPI interface.
type mbArtistAdapter struct {
	client *pipeline.MusicBrainzClient
}

func (a mbArtistAdapter) SearchArtistCandidates(ctx context.Context, name string) ([]catalog.MBArtistCandidate, error) {
	raw, err := a.client.SearchArtistCandidates(ctx, name)
	if err != nil {
		return nil, err
	}
	return toMBArtistCandidates(raw), nil
}

func (a mbArtistAdapter) LookupArtistURLs(ctx context.Context, mbid string) ([]string, error) {
	rels, err := a.client.LookupArtistURLRelations(ctx, mbid)
	if err != nil {
		return nil, err
	}
	return toURLResources(rels), nil
}

// toMBArtistCandidates maps MusicBrainz search results to the catalog enricher's
// candidate type.
func toMBArtistCandidates(raw []pipeline.MBArtistResult) []catalog.MBArtistCandidate {
	out := make([]catalog.MBArtistCandidate, 0, len(raw))
	for _, r := range raw {
		out = append(out, catalog.MBArtistCandidate{MBID: r.ID, Name: r.Name})
	}
	return out
}

// toURLResources flattens MusicBrainz url-relations to their resource URLs,
// dropping empty entries.
func toURLResources(rels []pipeline.MBURLRelation) []string {
	urls := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.URL.Resource != "" {
			urls = append(urls, r.URL.Resource)
		}
	}
	return urls
}

func printReport(r *catalog.CommonsEnrichReport) {
	if r == nil {
		return
	}
	fmt.Println("=== Summary ===")
	fmt.Printf("Artists:  scanned=%d matched=%d updated=%d skipped=%d errors=%d\n",
		r.ArtistsScanned, r.ArtistsMatched, r.ArtistsUpdated, r.ArtistsSkipped, r.ArtistErrors)
	fmt.Println()
	if r.ArtistErrors > 0 {
		fmt.Printf("%d artist%s errored and were left unchanged; re-run (idempotent) to retry them.\n",
			r.ArtistErrors, plural(r.ArtistErrors, "", "s"))
	}
	if r.DryRun {
		fmt.Println("DRY RUN — resolved Commons photos and reported matches; no DB writes.")
		fmt.Println("Re-run with --confirm to store the matched photo references.")
	} else {
		fmt.Println("LIVE — matched photo references committed.")
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
