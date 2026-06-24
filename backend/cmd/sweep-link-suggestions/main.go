// Command sweep-link-suggestions populates the artist_link_suggestions review
// queue (PSY-1199) for the bulk-backfill backlog (PSY-1206). It walks every
// link-less artist — bandcamp_embed_url IS NULL AND spotify IS NULL (~1859 rows
// at backfill time) — runs the MusicBrainz-backed DiscoverMusicService against
// each, and upserts the discovered Bandcamp/Spotify candidates as PENDING rows
// for an admin to triage in the PSY-1207 UI. NOTHING is auto-applied: the spikes
// (PSY-1196/1197) found false matches carry real links, so every row is
// human-reviewed.
//
// SEQUENTIAL + SHARED CLIENT (the reason this was gated on PSY-1208): the cmd
// builds EXACTLY ONE *MusicBrainzClient and ONE DiscoverMusicService, and
// processes artists STRICTLY SEQUENTIALLY through them. The single client's
// mutex serializes a ~1 req/s throttle that MusicBrainz enforces per IP (it
// BLOCKS for exceeding it). Parallel workers would each defeat that shared
// throttle — do NOT add them. See RunSweep for the full rationale.
//
// IDEMPOTENT / RESUMABLE: the upsert is ON CONFLICT (artist_id, platform, url) DO
// NOTHING, so a re-run inserts only new candidates and never resurrects an
// already-reviewed (accepted/rejected) row. Safe to interrupt and re-run.
//
// OPS NOTE: this is a SEPARATE process from the server, so while it runs it adds
// ~1 req/s to MusicBrainz on top of any server enrichment/discovery traffic
// (~2 req/s combined, briefly). Run it during low server traffic. Do NOT lower
// the throttle.
//
// Usage:
//
//	go run ./cmd/sweep-link-suggestions                  # dry-run (default)
//	go run ./cmd/sweep-link-suggestions --confirm        # upsert pending rows
//	go run ./cmd/sweep-link-suggestions --env .env.stage # target a specific env
//
// Dry-run (the default) reports the planned suggestion count and the scanned /
// skipped artist tallies WITHOUT writing anything.
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
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes — upsert pending suggestions (default: dry-run only)")
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
	fmt.Printf("=== Music-Link Suggestion Sweep (%s) — PSY-1206 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	// ONE shared MusicBrainz client → ONE discovery service. Every artist is
	// processed through this single service, whose mutex enforces the ~1 req/s MB
	// throttle process-wide (PSY-1208). See the package doc + RunSweep.
	mbClient := pipeline.NewMusicBrainzClient()
	disc := pipeline.NewDiscoverMusicService(database, mbClient)

	// The suggestion store (PSY-1199) owns the artist_link_suggestions table and
	// its ON CONFLICT upsert — the sweep only ORCHESTRATES, it doesn't touch the
	// store's persistence mechanics directly. UpsertSuggestions never invokes the
	// artist write path (only AcceptSuggestion does), but we wire the real artist
	// service to match the container's construction.
	store := pipeline.NewLinkSuggestionService(database, catalog.NewArtistService(database))

	// Cancel the sweep cleanly on SIGINT/SIGTERM so an interrupted run stops
	// between artists (and cancels the in-flight MB/liveness work) instead of
	// being hard-killed mid-write. Resumable: just re-run.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, err := RunSweep(ctx, database, disc, store, !confirm)
	if err != nil {
		log.Fatalf("sweep: %v", err)
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

func printReport(r *SweepReport) {
	if len(r.Errors) > 0 {
		fmt.Println("--- Errors (non-fatal; these artists were skipped) ---")
		for _, e := range r.Errors {
			fmt.Printf("  [ERROR] %s\n", e)
		}
		fmt.Println()
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Artists scanned (no bandcamp embed + no spotify): %d\n", r.ArtistsScanned)
	fmt.Printf("  with candidates:                                %d\n", r.ArtistsWithCandidates)
	fmt.Printf("  no candidates:                                  %d\n", r.ArtistsNoCandidates)
	fmt.Printf("Suggestions found (planned):                      %d\n", r.SuggestionsFound)
	if confirm {
		fmt.Printf("Suggestions written (new pending rows):           %d\n", r.SuggestionsWritten)
		fmt.Printf("Already present (skipped via ON CONFLICT):        %d\n", r.SuggestionsSkipped)
	}
	fmt.Printf("Errors:                                           %d\n", len(r.Errors))
	fmt.Println()

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to upsert pending suggestions.")
	} else {
		fmt.Println("LIVE — pending suggestions committed.")
	}

	// Exit non-zero if a live run hit errors so CI/cron wrappers can alert.
	if confirm && len(r.Errors) > 0 {
		os.Exit(1)
	}
}
