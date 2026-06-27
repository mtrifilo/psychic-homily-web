// Command apply-link-suggestions bulk-accepts artist_link_suggestion rows by ID,
// routing each through the real LinkSuggestionService.AcceptSuggestion path so
// the link is written to the artist (a Bandcamp accept triggers the PSY-1190
// profile→embed resolver via UpdateArtist) and the row is stamped accepted — no
// persistence logic is duplicated here.
//
// The CALLER decides WHICH ids to accept (e.g. a deterministic Bandcamp
// slug==normalized-name match, or an LLM triage pass over the PSY-1206 sweep
// queue) and passes them via --ids-file. This cmd only APPLIES them, safely and
// auditably. It NEVER rejects.
//
// IDEMPOTENT / REPLAY-SAFE: AcceptSuggestion is a no-op on an already-accepted
// row and returns a conflict on an already-rejected one (reported, never
// forced); non-pending rows are skipped before the call. Safe to interrupt and
// re-run.
//
// --ids-file format: one numeric id per line; blank lines and lines starting
// with '#' are ignored; duplicate ids are de-duped.
//
// Usage:
//
//	go run ./cmd/apply-link-suggestions --ids-file ids.txt --reviewer 2            # dry-run (default)
//	go run ./cmd/apply-link-suggestions --ids-file ids.txt --reviewer 2 --confirm  # apply
//	go run ./cmd/apply-link-suggestions --ids-file ids.txt --reviewer 2 --env /tmp/x.env
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	idsFile  string
	reviewer uint
	confirm  bool
	envFile  string
)

func main() {
	flag.StringVar(&idsFile, "ids-file", "", "Path to a file of artist_link_suggestion IDs, one per line (required)")
	flag.UintVar(&reviewer, "reviewer", 0, "Admin user ID to stamp as reviewed_by_user_id (required)")
	flag.BoolVar(&confirm, "confirm", false, "Apply changes — accept the suggestions (default: dry-run only)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	if idsFile == "" {
		log.Fatal("--ids-file is required")
	}
	if reviewer == 0 {
		log.Fatal("--reviewer (admin user ID) is required")
	}

	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}
	database := db.GetDB()

	ids, err := readIDs(idsFile)
	if err != nil {
		log.Fatalf("read ids: %v", err)
	}

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Apply Link Suggestions (%s) — %d id(s) ===\n", mode, len(ids))
	// Surface the resolved target so a mistargeted --confirm (e.g. prod via the
	// wrong --env) is caught before any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s  reviewer=%d\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL), reviewer)

	// The suggestion store owns AcceptSuggestion (writes the link via the artist
	// update path). Wire the real artist service to match the container.
	store := pipeline.NewLinkSuggestionService(database, catalog.NewArtistService(database))

	var accepted, skipped, errored int
	for _, id := range ids {
		// Load the row for reporting + a pre-flight status check (so a non-pending
		// row is reported clearly instead of bubbling through AcceptSuggestion).
		var s catalogm.ArtistLinkSuggestion
		if err := database.First(&s, id).Error; err != nil {
			fmt.Printf("  id %d: load failed: %v\n", id, err)
			errored++
			continue
		}
		var artist catalogm.Artist
		_ = database.Select("name").First(&artist, s.ArtistID).Error

		label := fmt.Sprintf("id %d  artist %d %q  %s  %s  [%s]",
			id, s.ArtistID, artist.Name, s.Platform, s.URL, s.Status)

		if s.Status != catalogm.LinkSuggestionStatusPending {
			fmt.Printf("  SKIP (not pending): %s\n", label)
			skipped++
			continue
		}

		if !confirm {
			fmt.Printf("  would ACCEPT: %s\n", label)
			accepted++
			continue
		}

		if _, err := store.AcceptSuggestion(id, reviewer); err != nil {
			fmt.Printf("  ERROR accepting %s: %v\n", label, err)
			errored++
			continue
		}
		fmt.Printf("  accepted: %s\n", label)
		accepted++
	}

	fmt.Printf("\n=== Summary (%s) ===\n", mode)
	if confirm {
		fmt.Printf("Accepted:               %d\n", accepted)
	} else {
		fmt.Printf("Would accept:           %d\n", accepted)
	}
	fmt.Printf("Skipped (not pending):  %d\n", skipped)
	fmt.Printf("Errors:                 %d\n", errored)
	if !confirm {
		fmt.Println("\nDRY RUN — no DB writes. Re-run with --confirm to accept.")
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
		if _, err := os.Stat(ef); err == nil {
			if err := godotenv.Load(ef); err != nil {
				log.Fatalf("load env file %s: %v", ef, err)
			}
			log.Printf("loaded env from %s", ef)
			return
		}
	}
}

// redactDBHost returns "host[/dbname]" from a DB URL, dropping any credentials,
// so the resolved target can be printed for confirmation without leaking the
// password.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparseable)"
	}
	if dbname := strings.TrimPrefix(u.Path, "/"); dbname != "" {
		return u.Host + "/" + dbname
	}
	return u.Host
}

// readIDs parses a file of one numeric id per line, ignoring blanks and
// '#'-comments and de-duplicating while preserving first-seen order.
func readIDs(path string) ([]uint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var ids []uint
	seen := make(map[uint]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", line, err)
		}
		id := uint(n)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
