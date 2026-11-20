// Command triage-artist-entities applies a reviewed list of decisions about
// artist entities that are not acts, or are several acts stored as one.
//
// The worklist is a CSV whose header names artist_slug, action and target,
// with optional show_slug and note columns; any other column is ignored. A file
// that still has a column whose name starts with "suggested_" is refused whole:
// the suggestions have to be reviewed into action and target first. action is
// one of:
//
//	rename  target = the new name. The name changes through
//	        ArtistService.UpdateArtist and the slug stays as it was, so the
//	        artist's URL keeps answering.
//	merge   target = the slug of the artist to keep. The row's artist is merged
//	        into it through ArtistService.MergeArtists: bills, releases,
//	        follows, tags and the rest move to the target, the row's name
//	        becomes an alias of the target, and the row's artist is deleted, so
//	        its URL answers 404.
//	remove  no target, or "title". The artist is not a performer: it is taken
//	        off every bill it is on and deleted, so its URL answers 404. With
//	        "title" its name is first written to the title of each of those
//	        shows; a show that already has a title refuses the row.
//	split   target = two or more act names separated by "|". On every bill the
//	        joined entity is on, its place is taken by the acts in order, each
//	        with the joined entity's role; an act is an existing artist of that
//	        name (matched without regard to case) or a new one. The joined
//	        entity is deleted, so its URL answers 404.
//	keep    no target; a recorded decision that changes nothing.
//
// show_slug, on a remove or split row, must name the only show the artist is
// billed on; leave it empty to act on every bill.
//
// Refused, reported, and the run goes on (exit status 1): an unknown artist or
// target slug; a merge into the artist itself; a rename onto a name or an
// alias another artist has; a remove or split of an artist with a release
// credit, a link, a label, a festival lineup, a matched radio play, a hand-made
// relationship or a relationship vote (it looks like a real act); a remove that
// would leave a show with no acts; an artist named on two rows; and rows whose
// writes depend on each other (see refuseEntangledRows), decided over the
// whole file so the result does not depend on row order.
//
// Usage:
//
//	go run ./cmd/triage-artist-entities --file reviewed.csv --actor 2 --env .env.x            # dry run (default)
//	go run ./cmd/triage-artist-entities --file reviewed.csv --actor 2 --env .env.x --confirm  # apply
//
// --actor names the admin user the writes are recorded against, the way the
// admin endpoints record them: a rename leaves a revision and an entity edit
// audit row, a merge leaves the merge_artists audit row, and a title written by
// a remove leaves a show revision. The admin endpoints record nothing for a
// bill change or an artist delete, and neither does this tool. --env is
// required on every run and the named file decides the database, so a dry run
// previews the same database the confirmed run with the same --env writes to.
//
// The dry run runs each row's write in a transaction it rolls back, so it
// prints what the write would do. --confirm applies each accepted row in its
// own transaction, in file order. A second run over the same file changes
// nothing: a renamed artist already has its name and a merge is recognised
// from its audit row, while a removed or split artist no longer exists, so its
// row is refused with a note that an earlier run may have applied it.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
)

func main() {
	var (
		file    string
		actorID uint
		confirm bool
		envFile string
	)
	flag.StringVar(&file, "file", "", "Path to the reviewed worklist CSV (required)")
	flag.UintVar(&actorID, "actor", 0, "ID of the admin user the writes are recorded against (required)")
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
	fmt.Printf("=== Triage Artist Entities (%s): %d row(s) from %s ===\n", mode, len(rows), file)
	// The resolved target is printed before anything runs, so a --confirm aimed
	// at the wrong database is visible. Credentials are not printed.
	fmt.Printf("Target: ENVIRONMENT=%q db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	if err := requireAdminActor(db.GetDB(), actorID); err != nil {
		log.Fatal(err)
	}

	if summary := runWorklist(db.GetDB(), rows, actorID, confirm, os.Stdout); summary.Refused() {
		os.Exit(1)
	}
}

// requireEnvFlag refuses a run with no --env. A dry run and a confirmed run both
// name their database, so a dry run cannot preview one database while the
// confirmed run writes to another.
func requireEnvFlag(envFile string) error {
	if strings.TrimSpace(envFile) == "" {
		return fmt.Errorf("--env is required on every run, dry run included: the file that names the database; " +
			"dry-run with the same --env you will confirm with")
	}
	return nil
}

// loadNamedEnv makes the file named by --env decide the database. It uses
// Overload, not Load, because Load skips keys already set in the process, so an
// exported DATABASE_URL would win over the file. A file without DATABASE_URL is
// refused rather than leaving the shell's or the default database in charge.
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

// redactDBHost returns "host[/dbname]" from a database URL, dropping the user
// and password, so the target can be printed for confirmation.
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
