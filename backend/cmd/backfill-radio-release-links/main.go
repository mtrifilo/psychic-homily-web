// Command backfill-radio-release-links re-evaluates every radio_plays.release_id
// under the radio matcher's release-link rule: a play with a linked artist keeps
// a release only when that release is credited to the artist (any
// artist_releases role); a play without one keeps it only when the album title
// matches exactly one catalog release whose only credited artist has the play's
// artist name. Links the rule refuses are cleared (release_id set to NULL) or,
// when the rule finds exactly one replacement, pointed at that release. Plays
// and releases are never deleted, and no column but release_id changes. Links
// an admin set through the link-play endpoint are re-evaluated too; the report
// lists every change to one in full.
//
// Usage:
//
//	go run ./cmd/backfill-radio-release-links                                # dry-run (default)
//	go run ./cmd/backfill-radio-release-links --out plan.csv                 # dry-run, full change list to a file
//	go run ./cmd/backfill-radio-release-links --confirm --out changes.csv    # apply changes
//	go run ./cmd/backfill-radio-release-links --verbose                      # every change on stdout
//	go run ./cmd/backfill-radio-release-links --limit 500                    # cap plays scanned
//	go run ./cmd/backfill-radio-release-links --env .env.stage               # target a specific env
//
// The dry run reads inside a READ ONLY transaction, so the server refuses any
// write. A live run needs --out: the full change list (play, outcome, the
// release it had, the release it gets) is written to that new file before any
// UPDATE, and a failed write aborts the run, so every change can be undone from
// the file. Evaluation and writes share one REPEATABLE READ transaction; if
// another session changes a scanned play meanwhile, nothing is applied and the
// run can be repeated.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/shared"
)

const (
	// removalSampleSize and relinkSampleSize bound the rows listed without
	// --verbose.
	removalSampleSize = 50
	relinkSampleSize  = 20
)

// runOptions are the parsed command-line choices run acts on.
type runOptions struct {
	confirm bool
	verbose bool
	limit   int
	// outPath, when set, receives the full change list as CSV. Required with
	// confirm.
	outPath string
}

func main() {
	var opts runOptions
	var envFile string
	flag.BoolVar(&opts.confirm, "confirm", false, "Apply changes (default: dry-run only); requires --out")
	flag.BoolVar(&opts.verbose, "verbose", false, "Print every removal and relink")
	flag.IntVar(&opts.limit, "limit", 0, "Max plays scanned, lowest id first (0 = all)")
	flag.StringVar(&opts.outPath, "out", "", "New file to receive every change as CSV (required with --confirm)")
	flag.StringVar(&envFile, "env", "", "Path to .env file; its values override the shell (defaults to .env.development / .env)")
	flag.Parse()

	if opts.confirm && opts.outPath == "" {
		log.Fatal("--confirm requires --out <new file>: the change list is the record needed to undo the run")
	}

	loadEnv(envFile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	mode := "DRY RUN"
	if opts.confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Radio Release Links Backfill (%s) ===\n", mode)
	// The resolved target prints before connecting; check it before trusting
	// a live run's output.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDatabaseURL(cfg.Database.URL))

	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	if err := run(db.GetDB(), opts, os.Stdout); err != nil {
		if shared.IsSerializationFailure(err) {
			log.Fatalf("backfill: another session changed a scanned play during the run; nothing was applied; re-run (%v)", err)
		}
		log.Fatalf("backfill: %v", err)
	}
}

// loadEnv loads an explicit --env file over the shell's environment, or else
// the first of .env.development / .env without overriding it.
func loadEnv(envFile string) {
	if envFile != "" {
		if err := godotenv.Overload(envFile); err != nil {
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

// run evaluates (and with confirm, applies) the backfill and prints the report.
func run(database *gorm.DB, opts runOptions, out io.Writer) error {
	backfill := catalog.RadioReleaseLinkBackfillOptions{DryRun: !opts.confirm, Limit: opts.limit}
	if opts.confirm {
		backfill.BeforeApply = func(r *catalog.RadioReleaseLinkReport) error {
			return writeChangeList(opts.outPath, r.Changes)
		}
	}
	report, err := catalog.BackfillRadioReleaseLinks(database, backfill)
	if err != nil {
		return err
	}
	if !opts.confirm && opts.outPath != "" {
		if err := writeChangeList(opts.outPath, report.Changes); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, formatReport(report, opts)); err != nil {
		if opts.confirm {
			return fmt.Errorf("changes are committed and listed in %s; printing the report failed: %w", opts.outPath, err)
		}
		return err
	}
	return nil
}

// writeChangeList writes every change as CSV to a file that must not exist
// yet, and syncs it to disk.
func writeChangeList(path string, changes []catalog.RadioReleaseLinkChange) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating change list: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("closing change list: %w", closeErr)
		}
	}()
	w := csv.NewWriter(f)
	rows := [][]string{{"play_id", "outcome", "artist_id", "from_release_id", "to_release_id", "admin_set"}}
	for _, c := range changes {
		artistID, toRelease := "", ""
		if c.ArtistID != nil {
			artistID = strconv.FormatUint(uint64(*c.ArtistID), 10)
		}
		if c.NewReleaseID != nil {
			toRelease = strconv.FormatUint(uint64(*c.NewReleaseID), 10)
		}
		rows = append(rows, []string{
			strconv.FormatUint(uint64(c.PlayID), 10), string(c.Outcome), artistID,
			strconv.FormatUint(uint64(c.CurrentReleaseID), 10), toRelease, strconv.FormatBool(c.AdminSet),
		})
	}
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("writing change list: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing change list: %w", err)
	}
	return nil
}

var outcomeLabels = map[catalog.RadioReleaseLinkOutcome]string{
	catalog.RadioReleaseLinkKeptCredited:           "Kept: release credited to the play's artist",
	catalog.RadioReleaseLinkKeptUniqueTitle:        "Kept: no artist link, unique title, sole credit has the play's artist name",
	catalog.RadioReleaseLinkRemovedNotCredited:     "Removed: release not credited to the play's artist, none with the title is",
	catalog.RadioReleaseLinkRemovedSeveralCredited: "Removed: release not credited to the play's artist, several with the title are",
	catalog.RadioReleaseLinkRemovedUnlinkedArtist:  "Removed: no artist link, rule picks no release (shared title, credits, or name)",
	catalog.RadioReleaseLinkRelinkedCredited:       "Relinked: to the one release with the title credited to the play's artist",
	catalog.RadioReleaseLinkRelinkedUniqueTitle:    "Relinked: no artist link, to the unique title whose sole credit matches",
}

func formatReport(r *catalog.RadioReleaseLinkReport, opts runOptions) string {
	out := &strings.Builder{}
	fmt.Fprintln(out, "=== Summary ===")
	fmt.Fprintf(out, "Plays with a release link scanned: %d\n", r.Scanned)
	for _, outcome := range catalog.RadioReleaseLinkOutcomes {
		fmt.Fprintf(out, "  %-82s %d\n", outcomeLabels[outcome]+":", r.Counts[outcome])
	}
	fmt.Fprintln(out)

	var adminSet []catalog.RadioReleaseLinkChange
	for _, c := range r.Changes {
		if c.AdminSet {
			adminSet = append(adminSet, c)
		}
	}
	fmt.Fprintf(out, "--- Changes to links an admin set (link_radio_play audit rows; all %d) ---\n", len(adminSet))
	writeChanges(out, adminSet)

	if opts.verbose {
		fmt.Fprintf(out, "--- Every removal and relink (%d) ---\n", len(r.Changes))
		writeChanges(out, r.Changes)
	} else {
		removalGroups := make([][]catalog.RadioReleaseLinkChange, 0, len(catalog.RadioReleaseLinkOutcomes))
		var relinks []catalog.RadioReleaseLinkChange
		removals := 0
		for _, outcome := range catalog.RadioReleaseLinkOutcomes {
			group := changesWithOutcome(r.Changes, outcome)
			switch {
			case outcome.IsRemoval():
				removalGroups = append(removalGroups, group)
				removals += len(group)
			case outcome.IsRelink():
				relinks = append(relinks, group...)
			}
		}
		sample := sampleByGroup(removalGroups, removalSampleSize)
		fmt.Fprintf(out, "--- Sample of removals (%d of %d) ---\n", len(sample), removals)
		writeChanges(out, sample)
		relinkSample := spread(relinks, min(len(relinks), relinkSampleSize))
		fmt.Fprintf(out, "--- Sample of relinks (%d of %d) ---\n", len(relinkSample), len(relinks))
		writeChanges(out, relinkSample)
	}

	switch {
	case opts.confirm:
		fmt.Fprintf(out, "LIVE: release links cleared %d, relinked %d. Every change is listed in %s.\n", r.Cleared, r.Relinked, opts.outPath)
	case opts.outPath != "":
		fmt.Fprintf(out, "DRY RUN: no DB writes. Planned changes are listed in %s. Re-run with --confirm --out <new file> to apply.\n", opts.outPath)
	default:
		fmt.Fprintln(out, "DRY RUN: no DB writes. Re-run with --confirm --out <new file> to apply.")
	}
	return out.String()
}

func writeChanges(out *strings.Builder, changes []catalog.RadioReleaseLinkChange) {
	for _, c := range changes {
		fmt.Fprintln(out, formatChange(c))
	}
	fmt.Fprintln(out)
}

func changesWithOutcome(changes []catalog.RadioReleaseLinkChange, outcome catalog.RadioReleaseLinkOutcome) []catalog.RadioReleaseLinkChange {
	var matched []catalog.RadioReleaseLinkChange
	for _, c := range changes {
		if c.Outcome == outcome {
			matched = append(matched, c)
		}
	}
	return matched
}

func formatChange(c catalog.RadioReleaseLinkChange) string {
	linked := "not linked"
	if c.ArtistID != nil {
		linked = fmt.Sprintf("linked: %q #%d", c.LinkedArtistName, *c.ArtistID)
	}
	credits := "(none)"
	if len(c.CurrentReleaseArtists) > 0 {
		credits = strings.Join(c.CurrentReleaseArtists, "; ")
	}
	line := fmt.Sprintf("  play %d [%s] artist %q (%s) album %q: release %d %q credited to %s",
		c.PlayID, c.Outcome, c.ArtistName, linked, c.AlbumTitle, c.CurrentReleaseID, c.CurrentReleaseTitle, credits)
	if c.NewReleaseID != nil {
		line += fmt.Sprintf(" -> release %d %q", *c.NewReleaseID, c.NewReleaseTitle)
	}
	return line
}

// sampleByGroup picks up to n changes, shared as evenly as the group sizes
// allow (a smaller group cedes its unused share to the others), each group's
// share spread evenly across its play-id order.
func sampleByGroup(groups [][]catalog.RadioReleaseLinkChange, n int) []catalog.RadioReleaseLinkChange {
	shares := make([]int, len(groups))
	for n > 0 {
		gave := false
		for i, g := range groups {
			if n > 0 && shares[i] < len(g) {
				shares[i]++
				n--
				gave = true
			}
		}
		if !gave {
			break
		}
	}
	var picked []catalog.RadioReleaseLinkChange
	for i, g := range groups {
		picked = append(picked, spread(g, shares[i])...)
	}
	return picked
}

// spread returns k items of xs at evenly spaced positions, first item included.
func spread(xs []catalog.RadioReleaseLinkChange, k int) []catalog.RadioReleaseLinkChange {
	picked := make([]catalog.RadioReleaseLinkChange, 0, k)
	for i := 0; i < k; i++ {
		picked = append(picked, xs[i*len(xs)/k])
	}
	return picked
}

// redactDatabaseURL prints a database URL as scheme://***@host:port/db: the
// userinfo becomes *** and the query and fragment are dropped. The host part
// is the text after the LAST '@', so a password holding '@' or '/' stays
// hidden. Empty prints "NOT SET". Input that does not reduce to that shape
// (no scheme, a '?' or '#' before the last '@', or a host part with characters
// outside letters, digits and ._:/-) prints "set (not shown)".
func redactDatabaseURL(raw string) string {
	const notShown = "set (not shown)"
	if raw == "" {
		return "NOT SET"
	}
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok || scheme == "" || !onlyChars(scheme, "+.-") {
		return notShown
	}
	userinfo := ""
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		if strings.ContainsAny(rest[:at], "?#") {
			return notShown
		}
		rest = rest[at+1:]
		userinfo = "***@"
	}
	if cut := strings.IndexAny(rest, "?#"); cut >= 0 {
		rest = rest[:cut]
	}
	if rest == "" || !onlyChars(rest, "._:/-") {
		return notShown
	}
	return scheme + "://" + userinfo + rest
}

// onlyChars reports whether s holds only ASCII letters, digits and extra.
func onlyChars(s, extra string) bool {
	for _, r := range s {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !isAlnum && !strings.ContainsRune(extra, r) {
			return false
		}
	}
	return true
}
