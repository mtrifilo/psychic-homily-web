// Command triage-artist-entities applies a reviewed list of decisions about
// artist entities that are not acts, or are several acts stored as one.
//
// Usage:
//
//	go run ./cmd/triage-artist-entities --env FILE --actor ID worklist.csv                          # dry run (default)
//	go run ./cmd/triage-artist-entities --env FILE --actor ID --dry-run worklist.csv                # the same, said out loud
//	go run ./cmd/triage-artist-entities --env FILE --actor ID --confirm --record FILE worklist.csv  # apply
//
// --env is required on every run, dry run included: the file names the
// database, overrides an exported DATABASE_URL, and is refused when it sets no
// DATABASE_URL. Dry-run with the same --env you will confirm with. The target
// (ENVIRONMENT and host/dbname, no credentials) is printed before any row runs.
//
// --actor names the active admin the writes are recorded against, the way the
// admin endpoints record them: a rename leaves a revision and an entity edit
// audit row, a merge leaves the merge_artists audit row, and a title written by
// a remove leaves a show revision. The admin endpoints record nothing for a
// bill change or an artist delete, and neither does this tool in the database.
//
// --confirm requires --record: a file opened for append (never truncated, a
// regular file readable by its owner only) that gets JSON lines stamped with
// the run, the actor and the database. Each applied row writes a "pending" line
// before its commit and a "committed" line after it; recordLine documents every
// outcome, how lines pair up, and what the pending line holds (what the row
// changes, its transaction id, and, for a merge, remove or split, a copy of the
// artist's row and of every row that referenced it, taken before the write).
// The pending line is synced to disk before the commit; if it cannot be, the
// row is rolled back and the run stops.
//
// The worklist is a CSV whose header names artist_slug, action and target, with
// optional show_slug, note, name and artist_id columns and the evidence
// columns listed in evidenceColumns, which are read past. Any other column
// refuses the file, and so does a column whose name starts with "suggested_":
// the suggestions have to be reviewed into action and target first. name and
// artist_id hold the artist as reviewed; a row whose artist now has another
// name, or whose slug now names another artist, is refused. action is one of:
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
// Refused, reported, and the run goes on: an unknown artist or target slug; a
// merge into the artist itself; a rename onto a name or an alias another artist
// has; a remove or split of an artist with a release credit, a link, a label, a
// festival lineup, a matched radio play, a hand-made relationship or a
// relationship vote (it looks like a real act); a remove that would leave a
// show with no acts; an artist named on two rows; and rows whose writes depend
// on each other (see refuseEntangledRows), decided over the whole file so the
// result does not depend on row order.
//
// The dry run runs each row's write in a transaction it rolls back, so it
// prints what the write would do. --confirm applies each accepted row in its
// own transaction, in file order. Every transaction has a 5 second lock
// timeout, a 60 second statement timeout and a 60 second idle timeout. A row
// that fails at its write is rolled back and the run stops there; so does a row
// whose record line or report output cannot be written. A row whose commit
// errors is settled by asking the database how its transaction ended; one it
// cannot settle is reported as OUTCOME UNKNOWN and stops the run. An interrupt
// lets the row in flight finish and stops the run before the next row (a
// second interrupt ends the process at once). Rename targets
// and split act names with invisible characters, non-plain spaces or doubled
// spaces are refused.
//
// A second run over the same file changes nothing: a renamed artist already
// has its name and a merge is recognised from its audit row, while a removed or
// split artist no longer exists, so its row is refused with a note that an
// earlier run may have applied it, and the run exits 1 by design.
//
// Exit status: 0 when no row was refused and the run did not stop (a dry run
// whose rows would all apply exits 0); 1 when any row was refused, a row's
// outcome is unknown, or the run stopped; 2 when the command line, the env
// file, the record file or the worklist cannot be used, or the actor is not an
// active admin; 3 when the configuration cannot be loaded, the database cannot
// be reached or read, or the report's header cannot be written.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
)

const (
	exitRowsRefused = 1
	exitUsage       = 2
	exitDatabase    = 3
)

func main() {
	confirm := flag.Bool("confirm", false, "Apply the worklist, each row in its own transaction (requires --record)")
	dryRun := flag.Bool("dry-run", false, "Preview the worklist and roll every row back (the default without --confirm)")
	envFile := flag.String("env", "", "Path to the .env file that names the database (required, on dry runs too)")
	recordPath := flag.String("record", "", "With --confirm: file the applied rows are appended to (required)")
	actorID := flag.Uint("actor", 0, "ID of the active admin the writes are recorded against (required)")
	flag.Usage = func() {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(),
			"usage: triage-artist-entities --env FILE --actor ID [--dry-run | --confirm --record FILE] WORKLIST.csv")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(exitUsage)
	}
	if err := validateFlags(*confirm, *dryRun, *envFile, *recordPath, *actorID); err != nil {
		usageFail("%v", err)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		usageFail("open worklist: %v", err)
	}
	rows, err := parseWorklist(f)
	_ = f.Close()
	if err != nil {
		usageFail("worklist %s: %s", clean(flag.Arg(0)), clean(err.Error()))
	}

	var rec *recorder
	if *confirm {
		record, err := openRecord(*recordPath)
		if err != nil {
			usageFail("%v", err)
		}
		defer func() { _ = record.Close() }()
		rec = &recorder{f: record, now: time.Now, actorID: *actorID,
			runID: time.Now().UTC().Format("20060102T150405.000000000Z")}
	}

	if err := loadNamedEnv(*envFile); err != nil {
		usageFail("%v", err)
	}
	log.Printf("loaded env from %s (authoritative)", *envFile)
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		fatal("connect db: %v", err)
	}
	// The shared connection logs slow and failed SQL to stdout with values
	// written in, which would put raw artist names between the report's lines.
	// Errors reach the report through the code instead.
	database := db.GetDB().Session(&gorm.Session{Logger: gormlogger.Discard})
	if rec != nil {
		rec.environment, rec.database = os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL)
	}

	// An interrupt lets the row in flight finish, record lines included, and
	// stops the run before the next row.
	// A second interrupt ends the process at once. A write to a closed pipe
	// (stdout piped to a tee that died) returns an error, which stops the run
	// through the report's write error, rather than killing the process before
	// the summary reaches stderr.
	signal.Ignore(syscall.SIGPIPE)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		interrupted.Store(true)
		signal.Stop(signals)
		_, _ = fmt.Fprintln(os.Stderr, "interrupt received: finishing the current row, then stopping; interrupt again to end at once")
	}()

	mode := "DRY RUN: every row runs and is rolled back"
	if *confirm {
		mode = "LIVE: each row commits on its own"
	}
	// The resolved target is printed before anything runs, so a run aimed at
	// the wrong database is visible. Credentials are not printed.
	header := fmt.Sprintf("=== Triage artist entities (%s): %d row(s) from %s ===\nTarget: ENVIRONMENT=%q db=%s\n\n",
		mode, len(rows), clean(flag.Arg(0)), os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	if _, err := os.Stdout.WriteString(header); err != nil {
		fatal("write the report: %v", err)
	}

	if err := requireAdminActor(database, *actorID); err != nil {
		if errors.Is(err, errNotAnActiveAdmin) {
			usageFail("%v", err)
		}
		fatal("%v", err)
	}

	outcome := runWorklist(database, rows, *actorID, rec, os.Stdout)
	if outcome.Stopped != nil {
		_, _ = fmt.Fprintf(os.Stderr, "the run stopped (%s). %s\n", clean(outcome.Stopped.Error()), outcome.appliedSentence())
	}
	if outcome.Failed() {
		os.Exit(exitRowsRefused)
	}
}

// validateFlags checks the command line before anything is opened or
// connected.
func validateFlags(confirm, dryRun bool, envFile, recordPath string, actorID uint) error {
	if confirm && dryRun {
		return errors.New("--dry-run and --confirm cannot be combined")
	}
	if err := requireEnvFlag(envFile); err != nil {
		return err
	}
	if actorID == 0 {
		return errors.New("--actor is required: the id of the admin user the writes are recorded against")
	}
	if confirm && strings.TrimSpace(recordPath) == "" {
		return errors.New("--confirm requires --record: the file the applied rows are appended to")
	}
	if !confirm && recordPath != "" {
		return errors.New("--record is written only by --confirm; a dry run applies nothing")
	}
	return nil
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

func usageFail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(exitUsage)
}

func fatal(format string, args ...any) {
	log.Printf(format, args...)
	os.Exit(exitDatabase)
}
