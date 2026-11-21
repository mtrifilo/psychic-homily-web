package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

// The --same-local-date worklist is a reattribute-shows worklist plus columns
// prefixed suggestedPrefix. reattribute-shows refuses a header it does not
// know, so the file cannot be run as written: a reviewer renames each
// suggested column by dropping the prefix, which yields columns that tool
// reads (merge_into_show_slug, and verdict, which it prints past).
const suggestedPrefix = "suggested_"

var sameLocalDateCSVHeader = []string{
	"show_slug",
	"correct_room_slug",
	"note",
	suggestedPrefix + "merge_into_show_slug",
	suggestedPrefix + "verdict",
}

// sameLocalDateCSVRows is one row per copy a group does not suggest keeping.
// correct_room_slug is the room the copy is already in, so a row whose merge
// column is left empty is a no-op in reattribute-shows ("already at the target
// room"), and a row whose merge column is filled merges the copy into the one
// it names.
func sameLocalDateCSVRows(groups []catalog.SameLocalDateGroup) [][]string {
	var out [][]string
	for _, g := range groups {
		_, suggestions := catalog.SuggestSameLocalDateKeep(g)
		for _, s := range suggestions {
			mergeInto := s.Keep.Slug
			if s.Action != catalog.SameLocalDateMerge {
				mergeInto = ""
			}
			out = append(out, []string{
				s.Copy.Slug,
				g.Room.Slug,
				sameLocalDateNote(g, s),
				mergeInto,
				fmt.Sprintf("%s (suggested): %s", s.Action, s.Reason),
			})
		}
	}
	return out
}

// sameLocalDateNote is the evidence for one row: the group, then this copy,
// then the copy it is compared with (the one suggested to keep on a merge row).
func sameLocalDateNote(g catalog.SameLocalDateGroup, s catalog.SameLocalDateSuggestion) string {
	other := "Other copy"
	if s.Action == catalog.SameLocalDateMerge {
		other = "Copy suggested to keep"
	}
	return fmt.Sprintf("%s at %s (%s, %s) on %s. This copy: %s. %s: %s.",
		g.HeadlinerName, g.Room.Name, g.Room.City, g.Room.State, g.LocalDate,
		copyEvidence(s.Copy), other, copyEvidence(s.Keep))
}

func copyEvidence(c catalog.SameLocalDateCopy) string {
	clock := c.LocalTime
	if c.DateOnlyAnchor {
		clock += " (20:00 date-only anchor, no music time stored)"
	}
	source := c.Source
	if c.DataSource != "" {
		source += "/" + c.DataSource
	}
	tickets := "no ticket link"
	if c.TicketURL != "" {
		tickets = "tickets " + c.TicketURL
	}
	return fmt.Sprintf("%s at %s, %s, created %s, source %s, %s, %d saves, %d comments",
		c.Slug, clock, c.EventDate.UTC().Format("2006-01-02T15:04:05Z"),
		c.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"), source, tickets, c.Saves, c.Comments)
}

// freeTextColumns are the worklist columns holding prose rather than a slug.
var freeTextColumns = map[int]bool{2: true, 4: true}

// writeSameLocalDateCSV writes the worklist. Every cell is cleaned, and the
// prose cells pass through spreadsheetSafe.
func writeSameLocalDateCSV(w io.Writer, groups []catalog.SameLocalDateGroup) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(sameLocalDateCSVHeader); err != nil {
		return err
	}
	for _, row := range sameLocalDateCSVRows(groups) {
		for i := range row {
			row[i] = clean(row[i])
			if freeTextColumns[i] {
				row[i] = spreadsheetSafe(row[i])
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// spreadsheetSafe keeps a prose cell that starts with an act's name from being
// read as a formula by a spreadsheet the reviewer opens the file in. Slug cells
// are left alone: reattribute-shows looks them up as written.
func spreadsheetSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}

// createWorklistFile opens path for a new worklist and refuses an existing
// file, so a worklist already under review is never overwritten.
func createWorklistFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("--csv %s: %w (the file is never overwritten; name a new one)", path, err)
	}
	return f, nil
}

// printSameLocalDateGroups prints every group with its copies and the
// suggestion for each copy not kept.
func printSameLocalDateGroups(w io.Writer, groups []catalog.SameLocalDateGroup) {
	_, _ = fmt.Fprintf(w, "Found %d group(s) of approved shows sharing a room, a venue-local date and a headliner.\n", len(groups))
	_, _ = fmt.Fprintf(w, "%s\n\n", catalog.SameLocalDateRuleText)
	for _, g := range groups {
		keep, suggestions := catalog.SuggestSameLocalDateKeep(g)
		_, _ = fmt.Fprintf(w, "%s  %s at %s [%s] (%s, %s)\n", g.LocalDate, clean(g.HeadlinerName),
			clean(g.Room.Name), clean(g.Room.Slug), clean(g.Room.City), clean(g.Room.State))
		for _, c := range g.Copies {
			mark := "    "
			if c.ShowID == keep.ShowID {
				mark = "  * "
			}
			_, _ = fmt.Fprintf(w, "%sshow %d  %s\n", mark, c.ShowID, clean(copyEvidence(c)))
		}
		for _, s := range suggestions {
			_, _ = fmt.Fprintf(w, "    suggested for %s: %s, %s\n", clean(s.Copy.Slug), s.Action, clean(s.Reason))
		}
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, "* marks the copy suggested to keep.")
}

// mainSameLocalDate is the --same-local-date mode: it lists, it never writes
// to the database. The worklist file is created before the database is read,
// so a name already taken fails before any work, and it is removed again if
// the run fails before the worklist is complete.
func mainSameLocalDate() {
	if err := validateSameLocalDateFlags(confirm, envFile); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var worklist *os.File
	if csvPath != "" {
		f, err := createWorklistFile(csvPath)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		worklist = f
	}
	fail := func(format string, args ...interface{}) {
		if worklist != nil {
			_ = worklist.Close()
			_ = os.Remove(csvPath)
		}
		log.Fatalf(format, args...)
	}

	if err := loadNamedEnv(envFile); err != nil {
		fail("%v", err)
	}
	log.Printf("loaded env from %s (authoritative)", envFile)
	cfg, err := config.Load()
	if err != nil {
		fail("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		fail("connect db: %v", err)
	}
	fmt.Printf("=== Shows sharing a room, a venue-local date and a headliner (READ ONLY) ===\n")
	fmt.Printf("Target: ENVIRONMENT=%q db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	groups, err := catalog.FindSameLocalDateGroups(db.GetDB())
	if err != nil {
		fail("%v", err)
	}
	printSameLocalDateGroups(os.Stdout, groups)
	if worklist == nil {
		return
	}
	if err := writeSameLocalDateCSV(worklist, groups); err != nil {
		fail("write %s: %v", csvPath, err)
	}
	if err := worklist.Close(); err != nil {
		worklist = nil
		_ = os.Remove(csvPath)
		log.Fatalf("close %s: %v", csvPath, err)
	}
	fmt.Printf("\nWrote %s. To merge: review every row and clear suggested_merge_into_show_slug on any "+
		"row that should not merge; then rename suggested_merge_into_show_slug to merge_into_show_slug and "+
		"suggested_verdict to verdict, and run reattribute-shows --dry-run with the same --env.\n", csvPath)
}

// validateSameLocalDateFlags checks the command line for the mode.
func validateSameLocalDateFlags(confirm bool, envFile string) error {
	if confirm {
		return errors.New("--same-local-date never writes; --confirm cannot be combined with it (merge with reattribute-shows)")
	}
	if strings.TrimSpace(envFile) == "" {
		return errors.New("--same-local-date requires --env: the file that names the database, " +
			"the same one reattribute-shows will run with")
	}
	return nil
}

// loadNamedEnv makes the file named by --env decide the database. It uses
// Overload, not Load, because Load skips keys already set in the process, so an
// exported DATABASE_URL would win over the file. A file without DATABASE_URL
// is refused rather than leaving the shell's or the default database in charge.
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
// credentials, so the target can be printed.
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

// clean replaces control and format characters (bidirectional overrides among
// them) and the Unicode line separators in text from the database, so a name
// cannot rewrite or reorder the terminal line or the CSV cell a reviewer reads.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return '?'
		}
		return r
	}, s)
}
