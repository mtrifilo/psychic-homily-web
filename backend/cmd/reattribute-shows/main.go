// Command reattribute-shows moves shows recorded at the wrong room onto the
// right one, from a human-reviewed CSV worklist.
//
// Usage:
//
//	go run ./cmd/reattribute-shows --env FILE worklist.csv                          # dry run (default)
//	go run ./cmd/reattribute-shows --env FILE --dry-run worklist.csv                # the same, said out loud
//	go run ./cmd/reattribute-shows --env FILE --confirm --record FILE worklist.csv  # apply
//
// --env is required on every run, dry run included: the file names the
// database, overrides an exported DATABASE_URL, and is refused when it sets no
// DATABASE_URL. Dry-run with the same --env you will confirm with. The target
// (ENVIRONMENT and host/dbname, no credentials) is printed before any row runs.
//
// --confirm requires --record: a file opened for append (never truncated) that
// gets one JSON line per applied row (when, the show, the room it left, the room
// it moved to, and for a merge the show kept and the show deleted). A room
// change writes no revision, so this file is the record of what the run did.
// Each line is written and synced to disk before its row commits.
//
// The worklist has a header row, and every column must be one this tool knows:
//
//	show_slug             the show to move (required)
//	correct_room_slug     the room it belongs in (required)
//	merge_into_show_slug  optional: a show already at that room, on the same
//	                      venue-local date, that is the same event. Both shows
//	                      must be approved. The row's show is merged into it and
//	                      deleted; the named show keeps its slug, headliner and
//	                      bill order, and the merged show's other acts join the
//	                      end of its bill in their own order, a headliner billed
//	                      as performer (every act as performer when the named
//	                      show's bill states no roles).
//	note                  optional, printed with the row
//
// plus the evidence columns listed in evidenceColumns, which are read past.
//
// A moved show keeps its slug, so its URL keeps answering and names the old
// room. A merged show's URL stops answering; the show it was merged into keeps
// its own.
//
// Rows run in file order. Under --confirm each row is its own transaction, with
// a 5 second lock timeout, a 60 second statement timeout and a 60 second idle
// timeout, so a dropped connection cannot leave show rows locked. A row that
// fails is rolled back and the run stops there; so does a row whose record line
// or report output cannot be written. A refused row writes nothing and the run
// goes on.
//
// The dry run holds the whole file in one transaction with the same timeouts,
// runs every row in a savepoint exactly as --confirm would, and rolls the lot
// back at the end, so a row that depends on an earlier one (a merge into a show
// an earlier row moved) is previewed against the state the earlier row leaves,
// and a row the database would reject is reported as failed. Until it rolls
// back, the dry run holds a lock on every row it wrote, and site writes to
// those rows wait; keep a worklist to a few dozen rows. Its output is printed
// after the rollback.
//
// A row is refused, before anything is written, when another show at the
// target room bills one of its acts at the same time: the database admits one
// of the two. A merged row run a second time finds no show and is refused, so
// re-running an applied worklist that holds merges exits 1 by design; its
// reason names the merge target that still exists.
//
// Exit status: 0 when every row applied or was already in place, 1 when any row
// was refused or failed or the run stopped, 2 when the command line, the env
// file, the record file or the worklist cannot be used, 3 when the
// configuration cannot be loaded, the database cannot be reached, or the dry
// run's transaction cannot be set up.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
)

const (
	exitRowsRefused = 1
	exitUsage       = 2
	exitDatabase    = 3
)

func main() {
	confirm := flag.Bool("confirm", false, "Apply the worklist, each row in its own transaction (requires --record)")
	dryRun := flag.Bool("dry-run", false, "Preview the worklist and roll it back (the default without --confirm)")
	envFile := flag.String("env", "", "Path to the .env file that names the database (required, on dry runs too)")
	recordPath := flag.String("record", "", "With --confirm: file the applied rows are appended to (required)")
	flag.Usage = func() {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(),
			"usage: reattribute-shows --env FILE [--dry-run | --confirm --record FILE] WORKLIST.csv")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(exitUsage)
	}
	if err := validateFlags(*confirm, *dryRun, *envFile, *recordPath); err != nil {
		usageFail("%v", err)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		usageFail("open worklist: %v", err)
	}
	rows, err := parseWorklist(f)
	_ = f.Close()
	if err != nil {
		usageFail("worklist %s: %v", flag.Arg(0), err)
	}

	var record *os.File
	if *confirm {
		if record, err = openRecord(*recordPath); err != nil {
			usageFail("%v", err)
		}
		defer func() { _ = record.Close() }()
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

	// The resolved target is printed before anything runs, so a run aimed at
	// the wrong database is visible. Credentials are not printed.
	target := fmt.Sprintf("Target: ENVIRONMENT=%q db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))
	if _, err := os.Stdout.WriteString(target); err != nil {
		fatal("write the report: %v", err)
	}

	database := db.GetDB()
	// A failed row reports the database's own error, which names the constraint
	// (and, through ReattributeShow, the key that collided), rather than the
	// bare sentinel GORM translates it to.
	database.TranslateError = false
	var tally runTally
	if *confirm {
		live := &report{w: os.Stdout}
		rec := &recorder{w: record, sync: record.Sync, now: time.Now}
		tally = run(live, rows, true, func(row worklistRow) (catalog.ShowReattributionResult, error) {
			return applyConfirmedRow(database, rec, row, transactionTimeouts)
		})
		if live.err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "the report could not be written (%v); the run stopped there. %s\n",
				live.err, tally.appliedSentence())
		}
	} else {
		var buf bytes.Buffer
		out := &report{w: &buf}
		err := database.Transaction(func(tx *gorm.DB) error {
			if err := setTimeouts(tx, transactionTimeouts); err != nil {
				return err
			}
			tally = run(out, rows, false, func(row worklistRow) (catalog.ShowReattributionResult, error) {
				return catalog.ReattributeShow(tx, row.Row), nil
			})
			return errDryRun
		})
		if !errors.Is(err, errDryRun) {
			fatal("dry run could not start: %v", err)
		}
		if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
			fatal("write the dry-run report: %v", err)
		}
	}
	if tally.refused+tally.failed > 0 || tally.stopped != nil {
		os.Exit(exitRowsRefused)
	}
}

// validateFlags checks the command line before anything is opened or
// connected.
func validateFlags(confirm, dryRun bool, envFile, recordPath string) error {
	if confirm && dryRun {
		return errors.New("--dry-run and --confirm cannot be combined")
	}
	if err := requireEnvFlag(envFile); err != nil {
		return err
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

// transactionTimeouts bound every transaction this tool opens: the dry run's
// one, and each confirmed row's. The lock timeout stops it waiting on a site
// write; the other two stop it holding its own locks when a statement runs away
// or the connection stops talking. None of them shortens how long a site write
// waits on a lock already held.
var transactionTimeouts = []string{
	"SET LOCAL lock_timeout = '5s'",
	"SET LOCAL statement_timeout = '60s'",
	"SET LOCAL idle_in_transaction_session_timeout = '60s'",
}

func setTimeouts(tx *gorm.DB, timeouts []string) error {
	for _, stmt := range timeouts {
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("set transaction timeouts: %w", err)
		}
	}
	return nil
}

// report writes the run's report and keeps the first write error; run stops at
// the next row once one is kept.
type report struct {
	w   io.Writer
	err error
}

func (r *report) f(format string, args ...interface{}) {
	if r.err != nil {
		return
	}
	_, r.err = fmt.Fprintf(r.w, format, args...)
}

// errDryRun rolls back the dry run's outer transaction.
var errDryRun = errors.New("dry run: roll back")

// reattributeFunc applies one row. A non-nil error stops the run after that
// row is reported; the row's own result says whether it was applied.
type reattributeFunc func(worklistRow) (catalog.ShowReattributionResult, error)

type runTally struct {
	moved, merged, unchanged, refused, failed int
	roomRowsCleared                           int64
	mergeSummary                              catalog.ShowDedupSummary
	// applied names every row that moved or merged, in file order.
	applied []string
	// stopped is why the run ended before the last row, or nil.
	stopped error
}

// appliedSentence says exactly which rows were applied, for a run that
// stopped or whose report could not be written.
func (t *runTally) appliedSentence() string {
	if len(t.applied) == 0 {
		return "No row was applied."
	}
	return fmt.Sprintf("Applied, and only these: %s.", strings.Join(t.applied, "; "))
}

// run applies, or previews, every row in file order and prints one block per
// row followed by the totals. It stops before the next row when reattribute
// returns an error or the report cannot be written. confirm only chooses the
// wording; what is written is decided by reattribute.
func run(w *report, rows []worklistRow, confirm bool, reattribute reattributeFunc) runTally {
	mode := "DRY RUN: the whole file runs and is rolled back"
	if confirm {
		mode = "LIVE: each row commits on its own"
	}
	w.f("=== Re-attribute shows (%s) ===\n\n", mode)

	var tally runTally
	for i, row := range rows {
		if w.err != nil {
			tally.stopped = fmt.Errorf("the report could not be written: %w", w.err)
			break
		}
		var res catalog.ShowReattributionResult
		var stop error
		if row.Refusal != "" {
			res = catalog.ShowReattributionResult{
				ShowSlug: row.Row.ShowSlug, Outcome: catalog.ReattributionRefused, Reason: row.Refusal}
		} else {
			res, stop = reattribute(row)
		}
		printResult(w, row, res, confirm)
		tally.count(row, res)
		if stop != nil {
			tally.stopped = stop
			w.f("STOPPED at line %d: %s\n", row.Line, clean(stop.Error()))
			if left := len(rows) - i - 1; left > 0 {
				w.f("%d row(s) after it were not run.\n", left)
			}
			w.f("\n")
			break
		}
	}
	printTotals(w, &tally, confirm)
	return tally
}

func (t *runTally) count(row worklistRow, res catalog.ShowReattributionResult) {
	switch res.Outcome {
	case catalog.ReattributionMoved:
		t.moved++
		t.applied = append(t.applied, fmt.Sprintf("line %d %s (moved)", row.Line, clean(res.ShowSlug)))
	case catalog.ReattributionMerged:
		t.merged++
		t.roomRowsCleared += res.RoomRowsCleared
		t.mergeSummary.Add(res.MergeSummary)
		t.applied = append(t.applied, fmt.Sprintf("line %d %s (merged into %s)",
			row.Line, clean(res.ShowSlug), clean(res.MergeIntoSlug)))
	case catalog.ReattributionUnchanged:
		t.unchanged++
	case catalog.ReattributionRefused:
		t.refused++
	default:
		t.failed++
	}
}

func printResult(w *report, row worklistRow, res catalog.ShowReattributionResult, confirm bool) {
	label := strings.ToUpper(string(res.Outcome))
	if !confirm && (res.Outcome == catalog.ReattributionMoved || res.Outcome == catalog.ReattributionMerged) {
		label = "WOULD BE " + label
	}
	w.f("line %d  %s  %s", row.Line, label, clean(res.ShowSlug))
	if res.ShowID != 0 {
		w.f(" (show %d, %s", res.ShowID, res.ShowStatus)
		if res.Headliner != "" {
			w.f(", %s", clean(res.Headliner))
		}
		if res.VenueLocalDate != "" {
			w.f(", %s", res.VenueLocalDate)
		}
		w.f(")")
	}
	w.f("\n")

	if res.CurrentRoom != nil || res.TargetRoom != nil {
		w.f("    room: %s -> %s\n", roomLabel(res.CurrentRoom), roomLabel(res.TargetRoom))
	}
	merging := res.MergeIntoSlug != ""
	switch {
	case res.MergeIntoID != 0:
		w.f("    merge into: %s (show %d, %s, %s)\n",
			clean(res.MergeIntoSlug), res.MergeIntoID, res.MergeIntoStatus, clean(res.MergeIntoHeadliner))
	case merging:
		w.f("    merge into: %s\n", clean(res.MergeIntoSlug))
	}
	switch res.Outcome {
	case catalog.ReattributionMoved:
		w.f("    slug: kept as %s (it names the old room)\n", clean(res.ShowSlug))
	case catalog.ReattributionMerged:
		w.f("    %s keeps its slug, headliner and bill order; %s stops answering\n",
			clean(res.MergeIntoSlug), clean(res.ShowSlug))
		for _, act := range res.ActsAddedToWinner {
			w.f("    act added to its bill: %s (%s, position %d)\n", clean(act.Name), act.SetType, act.Position)
		}
	}
	for _, n := range res.SameNightAtTarget {
		w.f("    WARNING: %s (show %d, %s, %s) is also at the target room that night; %s\n",
			clean(n.Slug), n.ID, n.Status, clean(n.Headliner), neighbourAdvice(n, merging))
	}
	if res.Reason != "" {
		w.f("    reason: %s\n", clean(res.Reason))
	}
	if row.Note != "" {
		w.f("    note: %s\n", clean(row.Note))
	}
	w.f("\n")
}

// neighbourAdvice says what a reviewer can do about another show at the target
// room that night. A merge needs both shows approved, so only an approved
// neighbour is offered as a merge target.
func neighbourAdvice(n catalog.ReattributionNeighbour, merging bool) string {
	switch {
	case n.Status != catalogm.ShowStatusApproved:
		return fmt.Sprintf("it is %s, so it cannot be merged with; moderate it first if it is the same event", n.Status)
	case merging:
		return "it stays after this merge; give it a row of its own if it is the same event"
	default:
		return "add it as merge_into_show_slug if it is the same event"
	}
}

func roomLabel(r *catalog.ReattributionRoom) string {
	if r == nil {
		return "?"
	}
	return fmt.Sprintf("%s [%s] (%s, %s)", clean(r.Name), clean(r.Slug), clean(r.City), clean(r.State))
}

// clean replaces control and format characters (bidirectional overrides among
// them) and the Unicode line separators in text that came from the database or
// the worklist, so a name cannot rewrite or reorder the terminal line the
// operator reviews from.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return '?'
		}
		return r
	}, s)
}

func printTotals(w *report, t *runTally, confirm bool) {
	verb := "would be"
	if confirm {
		verb = "were"
	}
	w.f("=== Totals ===\n")
	w.f("moved:     %d %s moved\n", t.moved, verb)
	w.f("merged:    %d %s merged\n", t.merged, verb)
	w.f("unchanged: %d already at their room\n", t.unchanged)
	w.f("refused:   %d\n", t.refused)
	w.f("failed:    %d\n", t.failed)
	if t.merged > 0 {
		w.f("merge references (from the merges above):\n")
		w.f("  %-22s %d\n", "RoomRowsCleared:", t.roomRowsCleared)
		printMergeSummary(w, &t.mergeSummary)
	}
	if confirm {
		if t.stopped != nil {
			w.f("\nThe run stopped early (%s). %s\n", clean(t.stopped.Error()), t.appliedSentence())
		}
		return
	}
	w.f("\nDRY RUN: nothing was written. Log lines printed above this report describe the rolled-back preview.\n")
	if t.stopped != nil {
		w.f("The preview stopped early: %s\n", clean(t.stopped.Error()))
	}
	w.f("Re-run with --confirm to apply.\n")
}

// mergeSummaryFieldsNotSet are ShowDedupSummary counters a reattribution merge
// never fills: it merges one named pair, not clusters, rewrites no slug, and
// clears the merged show's rooms before the merge, so the merge itself moves or
// skips no show_venues row (the cleared rows are printed on their own line).
var mergeSummaryFieldsNotSet = map[string]bool{
	"ClustersFound":     true,
	"SlugsRewritten":    true,
	"ShowVenuesMoved":   true,
	"ShowVenuesSkipped": true,
}

// printMergeSummary prints every counter in the summary except the ones a
// reattribution never fills, one per line, then the per-table reference counts
// in name order. It walks the struct rather than naming fields, so a counter
// added to ShowDedupSummary is printed without a change here.
func printMergeSummary(w *report, s *catalog.ShowDedupSummary) {
	v := reflect.ValueOf(*s)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i)
		if mergeSummaryFieldsNotSet[name] {
			continue
		}
		switch f.Kind() {
		case reflect.Int, reflect.Int64:
			w.f("  %-22s %d\n", name+":", f.Int())
		}
	}
	tables := make([]string, 0, len(s.EntityRefsMoved))
	for table := range s.EntityRefsMoved {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		w.f("  %-22s moved=%d skipped=%d\n", table+":", s.EntityRefsMoved[table], s.EntityRefsDropped[table])
	}
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

func usageFail(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(exitUsage)
}

func fatal(format string, args ...interface{}) {
	log.Printf(format, args...)
	os.Exit(exitDatabase)
}
