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
// name another room already has (see renameInTx and refuseCollidingRenames), a
// rename target shaped like a slug, a room named on two rows, a merge into a
// room another row merges away, and two merges into one room. Any refusal makes
// the exit status 1.
//
// Usage:
//
//	go run ./cmd/apply-venue-worklist --file reviewed.csv --actor 2 --env .env.x            # dry run (default)
//	go run ./cmd/apply-venue-worklist --file reviewed.csv --actor 2 --env .env.x --confirm  # apply
//
// --actor names the admin user every write is recorded against: the merge's
// audit row and a revision for each rename. --env is required on every run and
// the named file decides the database, so a dry run previews the same database
// the confirmed run with the same --env writes to.
//
// The dry run runs each merge and each rename in a transaction it rolls back,
// so it prints what the write would do, with both rooms' places, show counts
// and verification; a merge of a verified room into an unverified one is
// flagged, because the merge keeps the unverified room's fields. --confirm
// applies each accepted row in its own transaction. A second run over the same file changes nothing: a
// renamed room already has its name, and a merged room is recognised from the
// merge's audit record, which names the slug it removed. A confirmed run checks
// that record for every merge it applied and exits 1 if one is missing.
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
		actorID uint
		confirm bool
		envFile string
	)
	flag.StringVar(&file, "file", "", "Path to the reviewed worklist CSV (required)")
	flag.UintVar(&actorID, "actor", 0, "ID of the admin user the merges and renames are recorded against (required)")
	flag.BoolVar(&confirm, "confirm", false, "Apply the accepted rows (default: dry run only)")
	flag.StringVar(&envFile, "env", "", "Path to the .env file that names the database (required, on dry runs too)")
	flag.Parse()

	if file == "" {
		log.Fatal("--file is required")
	}
	if err := requireEnvFlag(envFile); err != nil {
		log.Fatal(err)
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

	if err := loadNamedEnv(envFile); err != nil {
		log.Fatal(err)
	}
	log.Printf("loaded env from %s (authoritative)", envFile)
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

	if err := requireAdminActor(db.GetDB(), actorID); err != nil {
		log.Fatal(err)
	}

	report := runWorklist(db.GetDB(), rows, actorID, confirm, os.Stdout)

	// Merge audit rows are written after each merge commits, on a background
	// writer; they are what a second run reads, so they are drained before exit
	// and then checked one by one.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := shared.ShutdownAuditWrites(ctx); err != nil {
		log.Printf("WARNING: audit writes did not finish: %v", err)
	}
	cancel()
	unrecorded := reportUnrecordedMerges(db.GetDB(), report, os.Stdout)

	if report.Summary.Refused() || unrecorded > 0 {
		os.Exit(1)
	}
}

// requireEnvFlag refuses a run with no --env. A dry run and a confirmed run both
// name their database, so a dry run cannot preview one database (whichever env
// file sits in the working directory, or the shell's) while the confirmed run
// writes to another.
func requireEnvFlag(envFile string) error {
	if strings.TrimSpace(envFile) == "" {
		return fmt.Errorf("--env is required on every run, dry run included: the file that names the database; " +
			"dry-run with the same --env you will confirm with")
	}
	return nil
}

// loadNamedEnv makes the file named by --env decide the database. It uses
// Overload, not Load, because Load skips keys already set in the process, so an
// exported DATABASE_URL would win over the file. Overload sets only the keys
// the file defines, so a file without DATABASE_URL is refused rather than
// leaving the shell's or the default database in charge.
func loadNamedEnv(envFile string) error {
	values, err := godotenv.Read(envFile)
	if err != nil {
		return fmt.Errorf("read env file %s: %w", envFile, err)
	}
	if strings.TrimSpace(values["DATABASE_URL"]) == "" {
		return fmt.Errorf("env file %s sets no DATABASE_URL; it must name the database", envFile)
	}
	if err := godotenv.Overload(envFile); err != nil {
		return fmt.Errorf("load env file %s: %w", envFile, err)
	}
	return nil
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
