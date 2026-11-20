// Command purge-replicated-shows removes year-replicated copies of approved
// shows: the same rooms, the same headliner, the same ticket URL and the same
// venue-local month and day, written into several venue-local years.
//
// Usage:
//
//	go run ./cmd/purge-replicated-shows --env <file>          # dry run (default): prints the plan and its digest
//	go run ./cmd/purge-replicated-shows --env <file> --confirm \
//	    --expect-plan <digest> --backup <new-file.jsonl>      # delete exactly the reviewed plan
//	go run ./cmd/purge-replicated-shows --list-beyond 24      # widen the far-future report
//
// Per group it keeps the copy in the earliest venue-local year and deletes the
// rest through the show-delete path, one transaction per group. A group is left
// untouched, and listed with the reason, unless all of these hold: its copies
// share one source, one submitter and one non-empty ticket URL; no two copies
// share a year; the earliest copy is in the current or the next venue-local
// year (a copy in an earlier year leaves the group untouched, which also means
// that after New Year a group whose original is in the year just ended is left
// for a human); at least one copy is past the far-future bound; and the copies
// to delete were created within 10 minutes of each other.
//
// --confirm deletes only when the freshly computed plan has the digest the
// reviewed dry run printed (--expect-plan), and writes every deleted show, with
// the rows that name it, to --backup (a new file, one JSON line per show)
// before deleting it.
//
// The far-future report lists approved shows dated more than --list-beyond
// months ahead that belong to no group. It never deletes anything.
//
// Running it again after a confirmed run finds nothing left to delete. Deleted
// pages keep serving from the frontend's cache until it revalidates; the run
// prints the deleted slugs for that.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

type options struct {
	confirm      bool
	expectPlan   string
	backupPath   string
	beyondMonths int
	now          time.Time
	target       string
}

func main() {
	var (
		opts    options
		envFile string
	)
	flag.BoolVar(&opts.confirm, "confirm", false, "Delete the planned copies (default: dry-run only)")
	flag.StringVar(&opts.expectPlan, "expect-plan", "", "With --confirm: the plan digest the reviewed dry run printed")
	flag.StringVar(&opts.backupPath, "backup", "", "With --confirm: a NEW file that receives every deleted show as one JSON line")
	flag.IntVar(&opts.beyondMonths, "list-beyond", catalog.ShowEventDateBoundMonths,
		"List approved shows dated more than this many months ahead that belong to no group (never deleted)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (authoritative; defaults to .env.development / .env)")
	flag.Parse()

	loadEnv(envFile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	opts.now = time.Now()
	opts.target = fmt.Sprintf("ENVIRONMENT=%q db=%s", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	os.Exit(run(db.GetDB(), opts, os.Stdout))
}

func loadEnv(envFile string) {
	if envFile != "" {
		// Overload, so the named file wins over a DATABASE_URL already exported
		// in the shell; the Target line run prints is the check that it did.
		if err := godotenv.Overload(envFile); err != nil {
			log.Fatalf("load env file %s: %v", envFile, err)
		}
		log.Printf("loaded env from %s (authoritative)", envFile)
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

// redactDBHost reduces a database URL to host and database name.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}

// run plans, prints, and on confirm deletes. It returns the process exit code:
// 0 on success, 1 when the options are refused, planning fails, the plan is not
// the expected one, or any group fails to delete.
func run(database *gorm.DB, opts options, w io.Writer) int {
	mode := "DRY RUN"
	if opts.confirm {
		mode = "LIVE"
	}
	say(w, "=== Purge year-replicated shows (%s) ===\n", mode)
	say(w, "Target: %s\n", orNone(opts.target))
	say(w, "Planned at %s\n\n", opts.now.UTC().Format(time.RFC3339))

	if opts.confirm && (opts.expectPlan == "" || opts.backupPath == "") {
		say(w, "[ERROR] --confirm needs --expect-plan <digest from the reviewed dry run> and --backup <new file>\n")
		return 1
	}

	plan, err := catalog.PlanReplicatedShowPurge(database, opts.now, opts.beyondMonths)
	if err != nil {
		say(w, "[ERROR] plan: %v\n", err)
		return 1
	}
	digest := plan.Digest()

	var backup *os.File
	if opts.confirm {
		if digest != opts.expectPlan {
			say(w, "[ERROR] the plan now has digest %s, not the reviewed %s; nothing deleted. Run the dry run again and review it.\n",
				digest, opts.expectPlan)
			return 1
		}
		backup, err = os.OpenFile(opts.backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			say(w, "[ERROR] backup file: %v; nothing deleted\n", err)
			return 1
		}
		defer func() { _ = backup.Close() }()
	}

	var actionable, untouched, planned, deleted, failedGroups int
	for _, g := range plan.Groups {
		if g.Actionable() {
			actionable++
			planned += len(g.DeleteIDs)
		} else {
			untouched++
		}
	}
	say(w, "Found %d replicated group(s): %d to purge (%d copies), %d left untouched.\n\n",
		len(plan.Groups), actionable, planned, untouched)

	var deletedSlugs []string
	for _, g := range plan.Groups {
		printGroup(w, g)
		if !opts.confirm || !g.Actionable() {
			say(w, "\n")
			continue
		}
		err := catalog.PurgeReplicatedShowGroup(database, opts.now, g, func(snapshot []json.RawMessage) error {
			return writeBackup(backup, g, snapshot)
		})
		if err != nil {
			failedGroups++
			say(w, "  [ERROR] nothing deleted in this group: %v\n\n", err)
			continue
		}
		deleted += len(g.DeleteIDs)
		for _, c := range g.Copies[1:] {
			deletedSlugs = append(deletedSlugs, c.Slug)
		}
		say(w, "  [DELETED] %d copies; kept show %d\n\n", len(g.DeleteIDs), g.KeepID)
	}

	printBeyond(w, plan)

	say(w, "=== Summary ===\n")
	say(w, "Groups found:        %d\n", len(plan.Groups))
	say(w, "Groups to purge:     %d\n", actionable)
	say(w, "Groups untouched:    %d\n", untouched)
	say(w, "Copies planned:      %d\n", planned)
	say(w, "Beyond %d months:    %d (listed, never deleted)\n", plan.BeyondMonths, len(plan.Beyond))
	say(w, "Plan digest:         %s\n", digest)
	if opts.confirm {
		say(w, "Copies deleted:      %d\n", deleted)
		say(w, "Groups failed:       %d\n", failedGroups)
		say(w, "Backup:              %s\n", opts.backupPath)
		say(w, "LIVE: deletions above are committed.\n")
		if len(deletedSlugs) > 0 {
			say(w, "Deleted slugs (their pages and the listings that link them serve from cache until revalidated):\n")
			for _, slug := range deletedSlugs {
				say(w, "  /shows/%s\n", clean(slug))
			}
		}
	} else {
		say(w, "DRY RUN: no database writes. To delete exactly this plan, re-run with\n")
		say(w, "  --confirm --expect-plan %s --backup <new file>\n", digest)
	}

	if failedGroups > 0 {
		return 1
	}
	return 0
}

// writeBackup appends one JSON line per show about to be deleted and syncs
// the file, so a line exists on disk before its show is gone.
func writeBackup(f *os.File, g catalog.ReplicatedShowGroup, snapshot []json.RawMessage) error {
	if f == nil {
		return errors.New("no backup file open")
	}
	for _, doc := range snapshot {
		line, err := json.Marshal(struct {
			KeptShowID uint            `json:"kept_show_id"`
			Deleted    json.RawMessage `json:"deleted"`
		}{g.KeepID, doc})
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return f.Sync()
}

func printGroup(w io.Writer, g catalog.ReplicatedShowGroup) {
	say(w, "Group: %s at %s, %s (venue-local month-day), current year %d\n",
		clean(g.HeadlinerName), clean(g.VenueName), g.MonthDay, g.CurrentYear)
	if g.Actionable() {
		say(w, "  Ticket URL: %s\n", clean(g.Copies[0].TicketURL))
	} else {
		say(w, "  UNTOUCHED: %s\n", g.SkipReason)
	}
	for _, c := range g.Copies {
		verdict := "    "
		if g.Actionable() {
			verdict = "DEL "
			if c.ShowID == g.KeepID {
				verdict = "KEEP"
			}
		}
		say(w, "  %s show %d  %s  %s  created %s  source=%s submitted_by=%s%s%s\n",
			verdict, c.ShowID, c.LocalDate, clean(c.Slug),
			c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			orNone(clean(c.Source)), submitter(c.SubmittedBy),
			refSuffix(c, verdict == "DEL "), ticketSuffix(g, c))
	}
}

func submitter(id uint) string {
	if id == 0 {
		return "(none)"
	}
	return fmt.Sprint(id)
}

// ticketSuffix prints each copy's own ticket URL only when the group is
// untouched, where differing URLs may be the reason.
func ticketSuffix(g catalog.ReplicatedShowGroup, c catalog.ReplicatedShowCopy) string {
	if g.Actionable() {
		return ""
	}
	return "  ticket=" + orNone(clean(c.TicketURL))
}

// refSuffix names the rows that point at a copy. Only a copy this run deletes
// says what the delete does with them; any other copy just counts them.
func refSuffix(c catalog.ReplicatedShowCopy, deleting bool) string {
	if len(c.Refs) == 0 {
		return ""
	}
	var all, drops, keeps []string
	for _, r := range c.Refs {
		entry := fmt.Sprintf("%s=%d", r.Table, r.Rows)
		all = append(all, entry)
		if r.Dropped {
			drops = append(drops, entry)
		} else {
			keeps = append(keeps, entry)
		}
	}
	if !deleting {
		return "  [refs " + strings.Join(all, " ") + "]"
	}
	out := ""
	if len(drops) > 0 {
		out += "  [deletes " + strings.Join(drops, " ") + "]"
	}
	if len(keeps) > 0 {
		out += "  [keeps as tombstones " + strings.Join(keeps, " ") + "]"
	}
	return out
}

func printBeyond(w io.Writer, plan *catalog.ReplicatedShowPlan) {
	say(w, "=== Shows dated after %s (more than %d months ahead) in no group: %d ===\n",
		plan.BeyondCutoff.Format(time.RFC3339), plan.BeyondMonths, len(plan.Beyond))
	say(w, "For a human decision; this command never deletes them.\n")
	for _, c := range plan.Beyond {
		say(w, "  show %d  %s  %s  %s at %s  created %s  source=%s submitted_by=%s  ticket=%s%s\n",
			c.ShowID, c.LocalDate, clean(c.Slug), clean(c.HeadlinerName), clean(c.VenueName),
			c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), orNone(clean(c.Source)), submitter(c.SubmittedBy),
			orNone(clean(c.TicketURL)), refSuffix(c, false))
	}
	say(w, "\n")
}

// clean makes a database string safe to print on the terminal a person reads
// before confirming: control characters (which can move the cursor and rewrite
// earlier lines) are dropped and the length is capped.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if runes := []rune(s); len(runes) > 200 {
		return string(runes[:200]) + "..."
	}
	return s
}

// say writes one line of the report. Report writes are best effort; the
// deletions they describe commit per group regardless.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
