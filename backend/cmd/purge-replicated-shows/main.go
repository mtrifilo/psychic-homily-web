// Command purge-replicated-shows removes year-replicated copies of approved
// shows: the same rooms, the same headliner, the same ticket URL and the same
// venue-local month and day, written into several venue-local years.
//
// Usage:
//
//	go run ./cmd/purge-replicated-shows                    # dry-run (default)
//	go run ./cmd/purge-replicated-shows --confirm          # delete the planned copies
//	go run ./cmd/purge-replicated-shows --list-beyond 24   # widen the far-future report
//
// Per group it keeps the copy in the earliest venue-local year that is not
// before the current one and deletes the rest through the show-delete path,
// one transaction per group. A group is left untouched, and listed with the
// reason, when its copies do not share one ticket URL, two copies share a year,
// a copy is in a year before the current one, or no copy is in the current or
// the next year.
//
// The far-future report lists approved shows dated more than --list-beyond
// months ahead that belong to no group. It never deletes anything.
//
// Running it again after a confirmed run finds nothing left to delete.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

type options struct {
	confirm      bool
	beyondMonths int
	now          time.Time
}

func main() {
	var (
		opts    options
		envFile string
	)
	flag.BoolVar(&opts.confirm, "confirm", false, "Delete the planned copies (default: dry-run only)")
	flag.IntVar(&opts.beyondMonths, "list-beyond", catalog.ShowEventDateBoundMonths,
		"List approved shows dated more than this many months ahead that belong to no group (never deleted)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
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
	os.Exit(run(db.GetDB(), opts, os.Stdout))
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
		if err := godotenv.Load(ef); err == nil {
			log.Printf("loaded env from %s", ef)
			return
		}
	}
	log.Println("no .env loaded; using process environment")
}

// run plans, prints, and on confirm deletes. It returns the process exit code:
// 0 on success, 1 when planning fails or any group fails to delete.
func run(database *gorm.DB, opts options, w io.Writer) int {
	mode := "DRY RUN"
	if opts.confirm {
		mode = "LIVE"
	}
	say(w, "=== Purge year-replicated shows (%s) ===\n", mode)
	say(w, "Planned at %s\n\n", opts.now.UTC().Format(time.RFC3339))

	plan, err := catalog.PlanReplicatedShowPurge(database, opts.now, opts.beyondMonths)
	if err != nil {
		say(w, "[ERROR] plan: %v\n", err)
		return 1
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

	for _, g := range plan.Groups {
		printGroup(w, g)
		if !opts.confirm || !g.Actionable() {
			say(w, "\n")
			continue
		}
		if err := catalog.PurgeReplicatedShowGroup(database, opts.now, g); err != nil {
			failedGroups++
			say(w, "  [ERROR] nothing deleted in this group: %v\n\n", err)
			continue
		}
		deleted += len(g.DeleteIDs)
		say(w, "  [DELETED] %d copies; kept show %d\n\n", len(g.DeleteIDs), g.KeepID)
	}

	printBeyond(w, plan)

	say(w, "=== Summary ===\n")
	say(w, "Groups found:        %d\n", len(plan.Groups))
	say(w, "Groups to purge:     %d\n", actionable)
	say(w, "Groups untouched:    %d\n", untouched)
	say(w, "Copies planned:      %d\n", planned)
	say(w, "Beyond %d months:    %d (listed, never deleted)\n", plan.BeyondMonths, len(plan.Beyond))
	if opts.confirm {
		say(w, "Copies deleted:      %d\n", deleted)
		say(w, "Groups failed:       %d\n", failedGroups)
		say(w, "LIVE: deletions above are committed.\n")
	} else {
		say(w, "DRY RUN: no database writes. Re-run with --confirm to delete the planned copies.\n")
	}

	if failedGroups > 0 {
		return 1
	}
	return 0
}

func printGroup(w io.Writer, g catalog.ReplicatedShowGroup) {
	say(w, "Group: %s at %s, %s (venue-local month-day), current year %d\n",
		g.HeadlinerName, g.VenueName, g.MonthDay, g.CurrentYear)
	if g.Actionable() {
		say(w, "  Ticket URL: %s\n", g.Copies[0].TicketURL)
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
		say(w, "  %s show %d  %s  %s  created %s%s%s\n",
			verdict, c.ShowID, c.LocalDate, c.Slug,
			c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			refSuffix(c), ticketSuffix(g, c))
	}
}

// ticketSuffix prints each copy's own ticket URL only when the group is
// untouched, where differing URLs may be the reason.
func ticketSuffix(g catalog.ReplicatedShowGroup, c catalog.ReplicatedShowCopy) string {
	if g.Actionable() {
		return ""
	}
	return "  ticket=" + orNone(c.TicketURL)
}

// refSuffix names the reference rows a copy carries, split by what DeleteShow
// does with them.
func refSuffix(c catalog.ReplicatedShowCopy) string {
	var drops, keeps []string
	for _, r := range c.Refs {
		entry := fmt.Sprintf("%s=%d", r.Table, r.Rows)
		if r.Dropped {
			drops = append(drops, entry)
		} else {
			keeps = append(keeps, entry)
		}
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
		say(w, "  show %d  %s  %s  %s at %s  created %s  ticket=%s%s\n",
			c.ShowID, c.LocalDate, c.Slug, c.HeadlinerName, c.VenueName,
			c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), orNone(c.TicketURL), refSuffix(c))
	}
	say(w, "\n")
}

// say writes one line of the report. A failed write to the report is not a
// reason to stop a run whose deletions are already committed per group.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
