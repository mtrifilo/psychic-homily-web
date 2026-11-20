// Command reattribute-shows moves shows recorded at the wrong room onto the
// right one, from a human-reviewed CSV worklist.
//
// Usage:
//
//	go run ./cmd/reattribute-shows worklist.csv            # dry run (default)
//	go run ./cmd/reattribute-shows --confirm worklist.csv  # apply
//
// The worklist has a header row. Columns are found by name and any others are
// ignored, so a worklist can carry its evidence:
//
//	show_slug             the show to move (required)
//	correct_room_slug     the room it belongs in; empty means "checked, leave it"
//	merge_into_show_slug  optional: a show already at that room that is the same
//	                      event. The row's show is merged into it and deleted.
//	note                  optional, printed with the row
//
// A moved show keeps its slug, so its URL keeps answering and names the old
// room. A merged show's URL stops answering; the show it was merged into keeps
// its own.
//
// Rows run in file order, each in its own transaction under --confirm. The dry
// run holds the whole file in one transaction, runs every row in a savepoint
// exactly as --confirm would, and rolls the lot back at the end. A row that
// depends on an earlier one (a merge into a show an earlier row moved) is
// therefore previewed against the state the earlier row leaves, and a row the
// database would reject is reported as failed. A refused or failed row is
// reported and the run goes on; the exit status is 1 when any row was refused
// or failed, 2 when the worklist itself cannot be read.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

func main() {
	confirm := flag.Bool("confirm", false, "Apply the worklist (default: dry run, the whole file rolled back)")
	envFile := flag.String("env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: reattribute-shows [--confirm] [--env FILE] WORKLIST.csv\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open worklist: %v\n", err)
		os.Exit(2)
	}
	rows, err := parseWorklist(f)
	_ = f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "worklist %s: %v\n", flag.Arg(0), err)
		os.Exit(2)
	}

	loadEnv(*envFile)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	database := db.GetDB()
	var tally runTally
	if *confirm {
		tally = run(os.Stdout, rows, true, func(r catalog.ShowReattribution) catalog.ShowReattributionResult {
			return catalog.ReattributeShow(database, r, true)
		})
	} else {
		err := database.Transaction(func(tx *gorm.DB) error {
			tally = run(os.Stdout, rows, false, func(r catalog.ShowReattribution) catalog.ShowReattributionResult {
				return catalog.ReattributeShow(tx, r, true)
			})
			return errDryRun
		})
		if !errors.Is(err, errDryRun) {
			log.Fatalf("dry run did not roll back cleanly: %v", err)
		}
	}
	if tally.refused+tally.failed > 0 {
		os.Exit(1)
	}
}

// errDryRun rolls back the dry run's outer transaction.
var errDryRun = errors.New("dry run: roll back")

// reattributeFunc applies one row. Under the dry run it runs inside the run's
// outer transaction, so its commit is a released savepoint.
type reattributeFunc func(catalog.ShowReattribution) catalog.ShowReattributionResult

type runTally struct {
	moved, merged, unchanged, skipped, refused, failed int
	mergeSummary                                       catalog.ShowDedupSummary
}

// run applies, or previews, every row in file order and prints one block per
// row followed by the totals. confirm only chooses the wording; what is written
// is decided by reattribute.
func run(w io.Writer, rows []worklistRow, confirm bool, reattribute reattributeFunc) runTally {
	mode := "DRY RUN: the whole file runs and is rolled back"
	if confirm {
		mode = "LIVE: each row commits on its own"
	}
	fmt.Fprintf(w, "=== Re-attribute shows (%s) ===\n\n", mode)

	var tally runTally
	for _, row := range rows {
		var res catalog.ShowReattributionResult
		switch {
		case row.Refusal != "":
			res = catalog.ShowReattributionResult{
				ShowSlug: row.Row.ShowSlug, Outcome: catalog.ReattributionRefused, Reason: row.Refusal}
		case row.Skip:
			fmt.Fprintf(w, "line %d  SKIPPED  %s: no target room given\n", row.Line, row.Row.ShowSlug)
			printNote(w, row.Note)
			fmt.Fprintln(w)
			tally.skipped++
			continue
		default:
			res = reattribute(row.Row)
		}
		printResult(w, row, res, confirm)
		tally.count(res)
	}
	printTotals(w, &tally, confirm)
	return tally
}

func (t *runTally) count(res catalog.ShowReattributionResult) {
	switch res.Outcome {
	case catalog.ReattributionMoved:
		t.moved++
	case catalog.ReattributionMerged:
		t.merged++
		t.mergeSummary.Add(res.MergeSummary)
	case catalog.ReattributionUnchanged:
		t.unchanged++
	case catalog.ReattributionRefused:
		t.refused++
	default:
		t.failed++
	}
}

func printResult(w io.Writer, row worklistRow, res catalog.ShowReattributionResult, confirm bool) {
	label := strings.ToUpper(string(res.Outcome))
	if !confirm && (res.Outcome == catalog.ReattributionMoved || res.Outcome == catalog.ReattributionMerged) {
		label = "WOULD BE " + label
	}
	fmt.Fprintf(w, "line %d  %s  %s", row.Line, label, res.ShowSlug)
	if res.ShowID != 0 {
		fmt.Fprintf(w, " (show %d", res.ShowID)
		if res.Headliner != "" {
			fmt.Fprintf(w, ", %s", res.Headliner)
		}
		if res.VenueLocalDate != "" {
			fmt.Fprintf(w, ", %s", res.VenueLocalDate)
		}
		fmt.Fprint(w, ")")
	}
	fmt.Fprintln(w)

	if res.CurrentRoom != nil || res.TargetRoom != nil {
		fmt.Fprintf(w, "    room: %s -> %s\n", roomLabel(res.CurrentRoom), roomLabel(res.TargetRoom))
	}
	switch res.Outcome {
	case catalog.ReattributionMoved:
		fmt.Fprintf(w, "    slug: kept as %s (it names the old room)\n", res.ShowSlug)
	case catalog.ReattributionMerged:
		fmt.Fprintf(w, "    merge into: %s (show %d), which keeps its slug; %s stops answering\n",
			res.MergeIntoSlug, res.MergeIntoID, res.ShowSlug)
		if len(res.ActsAddedToWinner) > 0 {
			fmt.Fprintf(w, "    acts added to %s: %s\n", res.MergeIntoSlug, strings.Join(res.ActsAddedToWinner, "; "))
		}
	}
	for _, n := range res.SameNightAtTarget {
		fmt.Fprintf(w, "    WARNING: %s (show %d, %s) is also at the target room that night; "+
			"add it as merge_into_show_slug if it is the same event\n", n.Slug, n.ID, n.Headliner)
	}
	if res.Reason != "" {
		fmt.Fprintf(w, "    reason: %s\n", res.Reason)
	}
	printNote(w, row.Note)
	fmt.Fprintln(w)
}

func printNote(w io.Writer, note string) {
	if note != "" {
		fmt.Fprintf(w, "    note: %s\n", note)
	}
}

func roomLabel(r *catalog.ReattributionRoom) string {
	if r == nil {
		return "?"
	}
	return fmt.Sprintf("%s [%s] (%s, %s)", r.Name, r.Slug, r.City, r.State)
}

func printTotals(w io.Writer, t *runTally, confirm bool) {
	verb := "would be"
	if confirm {
		verb = "were"
	}
	fmt.Fprintln(w, "=== Totals ===")
	fmt.Fprintf(w, "moved:     %d %s moved\n", t.moved, verb)
	fmt.Fprintf(w, "merged:    %d %s merged\n", t.merged, verb)
	fmt.Fprintf(w, "unchanged: %d already at their room\n", t.unchanged)
	fmt.Fprintf(w, "skipped:   %d with no target room\n", t.skipped)
	fmt.Fprintf(w, "refused:   %d\n", t.refused)
	fmt.Fprintf(w, "failed:    %d\n", t.failed)
	if t.merged > 0 {
		s := &t.mergeSummary
		fmt.Fprintln(w, "merge references (from the merges above):")
		fmt.Fprintf(w, "  show_artists moved=%d skipped=%d, show_reports moved=%d skipped=%d\n",
			s.ShowArtistsMoved, s.ShowArtistsSkipped, s.ShowReportsMoved, s.ShowReportsSkipped)
		fmt.Fprintf(w, "  revisions moved=%d, alerts moved=%d, notify jobs dropped=%d\n",
			s.RevisionsMoved, s.AlertRowsMoved, s.NotifyJobsDropped)
		tables := make([]string, 0, len(s.EntityRefsMoved))
		for table := range s.EntityRefsMoved {
			tables = append(tables, table)
		}
		sort.Strings(tables)
		for _, table := range tables {
			fmt.Fprintf(w, "  %s moved=%d dropped=%d\n", table, s.EntityRefsMoved[table], s.EntityRefsDropped[table])
		}
	}
	if !confirm {
		fmt.Fprintln(w, "\nDRY RUN: nothing was written. A merge's own log lines above describe the rolled-back preview.")
		fmt.Fprintln(w, "Re-run with --confirm to apply.")
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
		if err := godotenv.Load(ef); err == nil {
			log.Printf("loaded env from %s", ef)
			return
		}
	}
	log.Println("no .env loaded; using process environment")
}
