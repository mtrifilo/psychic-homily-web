// Command reattribute-shows moves shows recorded at the wrong room onto the
// right one, from a human-reviewed CSV worklist.
//
// Usage:
//
//	go run ./cmd/reattribute-shows worklist.csv            # dry run (default)
//	go run ./cmd/reattribute-shows --dry-run worklist.csv  # the same, said out loud
//	go run ./cmd/reattribute-shows --confirm worklist.csv  # apply
//
// The worklist has a header row. Columns are found by name and any others are
// ignored, so a worklist can carry its evidence:
//
//	show_slug             the show to move (required)
//	correct_room_slug     the room it belongs in (required)
//	merge_into_show_slug  optional: an approved show already at that room that is
//	                      the same event. The row's show is merged into it and
//	                      deleted; the named show keeps its slug, headliner and
//	                      bill order, and the merged show's other acts join the
//	                      end of its bill as they are.
//	note                  optional, printed with the row
//
// A moved show keeps its slug, so its URL keeps answering and names the old
// room. A merged show's URL stops answering; the show it was merged into keeps
// its own.
//
// Rows run in file order, each in its own transaction under --confirm. The dry
// run holds the whole file in one transaction, runs every row in a savepoint
// exactly as --confirm would, and rolls the lot back at the end, so a row that
// depends on an earlier one (a merge into a show an earlier row moved) is
// previewed against the state the earlier row leaves, and a row the database
// would reject is reported as failed. Until it rolls back, the dry run holds a
// lock on every row it wrote, and site writes to those rows wait; keep a
// worklist to a few dozen rows. Its output is printed after the rollback, and
// it gives up on a lock it would wait on for more than 5 seconds, on a
// statement running past 60 seconds, and when left idle for 60 seconds.
//
// A merged row run a second time finds no show and is refused, so re-running
// an applied worklist that holds merges exits 1; its reason names the merge
// target that still exists.
//
// Exit status: 0 when every row applied or was already in place, 1 when any row
// was refused or failed, 2 when the command line or the worklist cannot be
// used, 3 when the database cannot be reached or the dry run did not roll back.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

const (
	exitRowsRefused = 1
	exitUsage       = 2
	exitDatabase    = 3
)

func main() {
	confirm := flag.Bool("confirm", false, "Apply the worklist, each row in its own transaction")
	dryRun := flag.Bool("dry-run", false, "Preview the worklist and roll it back (the default without --confirm)")
	envFile := flag.String("env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: reattribute-shows [--dry-run | --confirm] [--env FILE] WORKLIST.csv\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(exitUsage)
	}
	if *confirm && *dryRun {
		fmt.Fprintln(os.Stderr, "--dry-run and --confirm cannot be combined")
		os.Exit(exitUsage)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open worklist: %v\n", err)
		os.Exit(exitUsage)
	}
	rows, err := parseWorklist(f)
	_ = f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "worklist %s: %v\n", flag.Arg(0), err)
		os.Exit(exitUsage)
	}

	loadEnv(*envFile)
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		fatal("connect db: %v", err)
	}

	database := db.GetDB()
	var tally runTally
	if *confirm {
		tally = run(os.Stdout, rows, true, func(r catalog.ShowReattribution) catalog.ShowReattributionResult {
			return catalog.ReattributeShow(database, r)
		})
	} else {
		var out bytes.Buffer
		err := database.Transaction(func(tx *gorm.DB) error {
			for _, stmt := range dryRunTimeouts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			tally = run(&out, rows, false, func(r catalog.ShowReattribution) catalog.ShowReattributionResult {
				return catalog.ReattributeShow(tx, r)
			})
			return errDryRun
		})
		if !errors.Is(err, errDryRun) {
			fatal("dry run did not roll back cleanly: %v", err)
		}
		_, _ = os.Stdout.Write(out.Bytes())
	}
	if tally.refused+tally.failed > 0 {
		os.Exit(exitRowsRefused)
	}
}

// dryRunTimeouts bound the dry run's one transaction. The lock timeout stops it
// waiting on a site write; the other two stop it holding its own locks when a
// statement runs away or the process stops reading. None of them shortens how
// long a site write waits on a lock the dry run already holds.
var dryRunTimeouts = []string{
	"SET LOCAL lock_timeout = '5s'",
	"SET LOCAL statement_timeout = '60s'",
	"SET LOCAL idle_in_transaction_session_timeout = '60s'",
}

// errDryRun rolls back the dry run's outer transaction.
var errDryRun = errors.New("dry run: roll back")

// reattributeFunc applies one row. Under the dry run it runs inside the run's
// outer transaction, as a savepoint.
type reattributeFunc func(catalog.ShowReattribution) catalog.ShowReattributionResult

type runTally struct {
	moved, merged, unchanged, refused, failed int
	mergeSummary                              catalog.ShowDedupSummary
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
		if row.Refusal != "" {
			res = catalog.ShowReattributionResult{
				ShowSlug: row.Row.ShowSlug, Outcome: catalog.ReattributionRefused, Reason: row.Refusal}
		} else {
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
	fmt.Fprintf(w, "line %d  %s  %s", row.Line, label, clean(res.ShowSlug))
	if res.ShowID != 0 {
		fmt.Fprintf(w, " (show %d, %s", res.ShowID, res.ShowStatus)
		if res.Headliner != "" {
			fmt.Fprintf(w, ", %s", clean(res.Headliner))
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
	if res.MergeIntoID != 0 {
		fmt.Fprintf(w, "    merge into: %s (show %d, %s, %s)\n",
			clean(res.MergeIntoSlug), res.MergeIntoID, res.MergeIntoStatus, clean(res.MergeIntoHeadliner))
	}
	switch res.Outcome {
	case catalog.ReattributionMoved:
		fmt.Fprintf(w, "    slug: kept as %s (it names the old room)\n", clean(res.ShowSlug))
	case catalog.ReattributionMerged:
		fmt.Fprintf(w, "    %s keeps its slug, headliner and bill order; %s stops answering\n",
			clean(res.MergeIntoSlug), clean(res.ShowSlug))
		for _, act := range res.ActsAddedToWinner {
			fmt.Fprintf(w, "    act added to its bill: %s (%s, position %d)\n", clean(act.Name), act.SetType, act.Position)
		}
	}
	for _, n := range res.SameNightAtTarget {
		fmt.Fprintf(w, "    WARNING: %s (show %d, %s, %s) is also at the target room that night; "+
			"add it as merge_into_show_slug if it is the same event\n", clean(n.Slug), n.ID, n.Status, clean(n.Headliner))
	}
	if res.Reason != "" {
		fmt.Fprintf(w, "    reason: %s\n", clean(res.Reason))
	}
	if row.Note != "" {
		fmt.Fprintf(w, "    note: %s\n", clean(row.Note))
	}
	fmt.Fprintln(w)
}

func roomLabel(r *catalog.ReattributionRoom) string {
	if r == nil {
		return "?"
	}
	return fmt.Sprintf("%s [%s] (%s, %s)", clean(r.Name), clean(r.Slug), clean(r.City), clean(r.State))
}

// clean replaces control characters in text that came from the database or the
// worklist, so a name cannot rewrite the terminal the operator reviews from.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
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
	fmt.Fprintf(w, "refused:   %d\n", t.refused)
	fmt.Fprintf(w, "failed:    %d\n", t.failed)
	if t.merged > 0 {
		fmt.Fprintln(w, "merge references (from the merges above):")
		printMergeSummary(w, &t.mergeSummary)
	}
	if !confirm {
		fmt.Fprintln(w, "\nDRY RUN: nothing was written. Log lines printed above this report describe the rolled-back preview.")
		fmt.Fprintln(w, "Re-run with --confirm to apply.")
	}
}

// mergeSummaryFieldsNotSet are ShowDedupSummary counters a reattribution merge
// never fills: it merges one named pair, not clusters, and rewrites no slug.
var mergeSummaryFieldsNotSet = map[string]bool{"ClustersFound": true, "SlugsRewritten": true}

// printMergeSummary prints every counter in the summary except the ones a
// reattribution never fills, one per line, then the per-table reference counts
// in name order. It walks the struct rather than naming fields, so a counter
// added to ShowDedupSummary is printed without a change here.
func printMergeSummary(w io.Writer, s *catalog.ShowDedupSummary) {
	v := reflect.ValueOf(*s)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i)
		if mergeSummaryFieldsNotSet[name] {
			continue
		}
		switch f.Kind() {
		case reflect.Int, reflect.Int64:
			fmt.Fprintf(w, "  %-22s %d\n", name+":", f.Int())
		}
	}
	tables := make([]string, 0, len(s.EntityRefsMoved))
	for table := range s.EntityRefsMoved {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		fmt.Fprintf(w, "  %-22s moved=%d skipped=%d\n", table+":", s.EntityRefsMoved[table], s.EntityRefsDropped[table])
	}
}

func fatal(format string, args ...interface{}) {
	log.Printf(format, args...)
	os.Exit(exitDatabase)
}

func loadEnv(envFile string) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "load env file %s: %v\n", envFile, err)
			os.Exit(exitUsage)
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
