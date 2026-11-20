// Command apply-venue-worklist applies a reviewed list of decisions about venue
// rooms: merge a room into another room of the same city and state, rename a
// room, or record that a room keeps its name.
//
// The worklist is a CSV whose header names room_slug, action and target, with
// an optional note column; any other column is ignored. action is one of:
//
//	merge   target = the slug of the room to keep; the row's room is merged into
//	        it through VenueService.MergeVenuesInSamePlace and deleted, so the
//	        row's room URL answers 404 afterwards
//	rename  target = the new name; the name changes through
//	        VenueService.UpdateVenue and the slug stays exactly as it was
//	keep    no target; a recorded decision that changes nothing
//
// Refused, reported, and the run continues: an unknown room or target slug, a
// merge into the room itself, a merge across cities or states, a rename onto a
// name another room of the same city and state already has, a room named on
// two rows, and a merge into a room another row merges away. Any refusal makes
// the exit status 1.
//
// Usage:
//
//	go run ./cmd/apply-venue-worklist --file reviewed.csv              # dry run (default)
//	go run ./cmd/apply-venue-worklist --file reviewed.csv --confirm    # apply
//	go run ./cmd/apply-venue-worklist --file reviewed.csv --env .env.x # pick a target
//
// The dry run runs each merge and each rename in a transaction it rolls back,
// so it prints what the write would do. --confirm applies each accepted row in
// its own transaction. A second run over the same file changes nothing: a
// renamed room already has its name, and a merged room is recognised from the
// merge's audit record, which names the slug it removed.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/shared"
)

func main() {
	var (
		file    string
		confirm bool
		envFile string
	)
	flag.StringVar(&file, "file", "", "Path to the reviewed worklist CSV (required)")
	flag.BoolVar(&confirm, "confirm", false, "Apply the accepted rows (default: dry run only)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	if file == "" {
		log.Fatal("--file is required")
	}
	f, err := os.Open(file)
	if err != nil {
		log.Fatalf("open worklist: %v", err)
	}
	rows, err := parseWorklist(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("worklist %s: %v", file, err)
	}

	loadEnv(envFile)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Apply Venue Worklist (%s): %d row(s) from %s ===\n", mode, len(rows), file)
	// The resolved target is printed before anything runs, so a --confirm aimed
	// at the wrong database is visible. Credentials are not printed.
	fmt.Printf("Target: ENVIRONMENT=%q db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	summary := runWorklist(db.GetDB(), rows, confirm, os.Stdout)

	// Merge audit rows are written after each merge commits, on a background
	// writer; they are what a second run reads, so they are drained before exit.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := shared.ShutdownAuditWrites(ctx); err != nil {
		log.Printf("WARNING: audit writes did not finish: %v", err)
	}
	cancel()

	if summary.Refused() {
		os.Exit(1)
	}
}

func loadEnv(envFile string) {
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
	log.Println("no .env loaded; using process environment")
}

// redactDBHost returns "host[/dbname]" from a database URL, dropping the
// credentials, so the target can be printed for confirmation.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparseable)"
	}
	if name := strings.TrimPrefix(u.Path, "/"); name != "" {
		return u.Host + "/" + name
	}
	return u.Host
}
