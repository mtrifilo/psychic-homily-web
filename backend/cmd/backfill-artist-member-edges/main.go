// Command backfill-artist-member-edges derives member_of / side_project edges
// from MusicBrainz artist-rels for artists that have a musicbrainz_artist_id
// (PSY-1382).
//
// Mapping (documented in catalog/mb_artist_rels.go):
//
//	"member of band" → member_of
//	"is person"      → side_project (performance-name / legal-name links)
//
// Ended memberships are included. Peers without a local MBID match are skipped.
// A full run (no --limit) upserts and reconciles via PSY-1332; a --limit run
// upserts only (so it cannot wipe edges outside the sample).
//
// Usage:
//
//	go run ./cmd/backfill-artist-member-edges                 # dry-run (default)
//	go run ./cmd/backfill-artist-member-edges --confirm       # apply + full reconcile
//	go run ./cmd/backfill-artist-member-edges --limit 20      # sample look-ups (dry-run)
//	go run ./cmd/backfill-artist-member-edges --env .env.stage
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/mbadapter"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm bool
	limit   int
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.IntVar(&limit, "limit", 0, "Max MBID artists to look up (0 = all). Partial confirm upserts without stale-reconcile.")
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
	fmt.Printf("=== Artist Member / Side-Project Edges from MusicBrainz (%s) — PSY-1382 ===\n", mode)
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	if limit > 0 {
		fmt.Printf("Limit: %d artists (lookup); peer resolution uses full MBID map\n", limit)
	}
	fmt.Println()

	mb := pipeline.NewMusicBrainzClient()
	svc := catalog.NewArtistRelationshipService(database)
	svc.SetArtistRelsClient(mbadapter.NewArtistRelsAdapter(mb))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result, err := svc.DeriveMusicBrainzArtistRelsWithOptions(ctx, catalog.MusicBrainzArtistRelsOptions{
		Limit:  limit,
		DryRun: !confirm,
	})
	if err != nil {
		log.Fatalf("derive: %v", err)
	}

	fmt.Printf("Artists scanned:       %d\n", result.ArtistsScanned)
	fmt.Printf("Lookups failed:        %d\n", result.LookupsFailed)
	fmt.Printf("Peers skipped:         %d\n", result.PeersSkipped)
	fmt.Printf("member_of upserted:    %d\n", result.MemberOfUpserted)
	fmt.Printf("side_project upserted: %d\n", result.SideProjectUpserted)
	if !confirm {
		fmt.Println("\nDry-run only — re-run with --confirm to write.")
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
}

func redactDBHost(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "(unparsed)"
	}
	return u.Host + u.Path
}
