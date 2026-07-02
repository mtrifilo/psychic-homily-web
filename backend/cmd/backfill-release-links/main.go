// Command backfill-release-links fills missing bandcamp/spotify external links on
// releases from MusicBrainz release-level url-rels (PSY-1307). For every release
// with a stored release-group MBID (PSY-1281/1282) it browses that RG's releases
// with url-rels — identity-verified through the MBID chain, so no homonym risk —
// host-anchors the URLs (bandcamp /album|/track, spotify /album|/track), and adds
// ONE link per missing platform through ReleaseService.AddExternalLink (which also
// back-fills credited artists' NULL bandcamp embeds, PSY-1189).
//
// Usage:
//
//	go run ./cmd/backfill-release-links                  # dry-run (default)
//	go run ./cmd/backfill-release-links --confirm        # apply changes
//	go run ./cmd/backfill-release-links --verbose        # per-fill detail
//	go run ./cmd/backfill-release-links --limit 50       # cap releases scanned
//	go run ./cmd/backfill-release-links --env .env.stage # target a specific env
//
// Dry-run writes nothing and prints the planned fills. Fill-when-empty per
// platform: a release that already has a bandcamp (or spotify) link keeps it.
// Re-run behavior: a release drops out of the candidate set only when BOTH
// platforms are linked — single-platform releases AND releases whose RG carried
// no usable url-rel (~30% by the spike) are re-browsed on every run. The
// PSY-1316 no-result memo exists but this cmd is deliberately memo-AGNOSTIC
// (ReattemptWindow=0): it neither filters on nor stamps
// links_enrich_attempted_at, so a manual run re-visits everything and leaves
// the sweep's convergence state untouched. MusicBrainz browse is ~1 req/s, one
// browse (up to 10 paginated calls) per distinct release-group, so a full run
// over the catalog is long — run detached. Concurrent live runs are safe for
// duplicates (writes carry source='mb_backfill', covered by a partial unique
// index; a collision counts as "raced") but wasteful — they browse the same RGs.
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
	"psychic-homily-backend/internal/services/enrich"
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
	flag.BoolVar(&verbose, "verbose", false, "Print per-fill detail")
	flag.IntVar(&limit, "limit", 0, "Max releases to scan (0 = all)")
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
	fmt.Printf("=== Release Links Backfill (%s) — PSY-1307 ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	fmt.Printf("Source: MusicBrainz release browse-by-RG-MBID (inc=url-rels), host-anchored bandcamp/spotify\n\n")

	report, err := enrich.BackfillReleaseLinks(
		database,
		pipeline.NewMusicBrainzClient(),
		catalog.NewReleaseService(database),
		enrich.ReleaseLinksOptions{
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

// redactDBHost extracts host[:port]/dbname from a database URL, dropping any
// embedded credentials, so the target can be logged without leaking secrets.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}

func printReport(r *enrich.ReleaseLinksReport) {
	// Fills print unconditionally: a convenient audit trail of what was written
	// (rows also carry source='mb_backfill' in the DB — PSY-1316).
	header := "--- Planned fills ---"
	if confirm {
		header = "--- Fills written ---"
	}
	fmt.Println(header)
	if len(r.Fills) == 0 {
		fmt.Println("  (none)")
	}
	for _, f := range r.Fills {
		fmt.Printf("  [%s] release %d %q <- %s\n", f.Platform, f.ReleaseID, f.ReleaseTitle, f.URL)
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
	fmt.Printf("Releases scanned (RG-MBID, link gap):   %d\n", r.ReleasesScanned)
	fmt.Printf("Release-groups browsed (MB):            %d\n", r.RGsBrowsed)
	fmt.Printf("  bandcamp links filled:               %d\n", r.FilledBandcamp)
	fmt.Printf("  spotify links filled:                %d\n", r.FilledSpotify)
	fmt.Printf("  releases with no usable url-rel:     %d\n", r.ReleasesNoLinks)
	fmt.Printf("  releases skipped (RG browse failed): %d\n", r.ReleasesSkippedFailedRG)
	fmt.Printf("  links raced (already present at write): %d\n", r.LinksRaced)
	fmt.Printf("Errors:                                %d\n", len(r.Errors))
	fmt.Println()

	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	} else {
		fmt.Println("LIVE — changes committed.")
	}

	// Exit non-zero on errors in ANY mode: a dry-run is the review gate before
	// --confirm, so a partially-failed plan must not read as success to wrappers.
	if len(r.Errors) > 0 {
		os.Exit(1)
	}
}
