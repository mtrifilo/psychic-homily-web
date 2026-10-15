// Command backfill-radio-release-links re-evaluates every radio_plays.release_id
// under the radio matcher's release-link rule: a play with a linked artist keeps
// a release only when that release is credited to the artist (any
// artist_releases role); a play without one keeps it only when the album title
// matches exactly one catalog release whose only credited artist has the play's
// artist name. Links the rule refuses are cleared (release_id set to NULL) or,
// when the rule finds another release, pointed at that release. A link an admin
// set through the link-play endpoint (audit action link_radio_play) is kept.
// Plays and releases are never deleted, and no column but release_id changes.
//
// Usage:
//
//	go run ./cmd/backfill-radio-release-links                  # dry-run (default)
//	go run ./cmd/backfill-radio-release-links --confirm        # apply changes
//	go run ./cmd/backfill-radio-release-links --verbose        # every change
//	go run ./cmd/backfill-radio-release-links --limit 500      # cap plays scanned
//	go run ./cmd/backfill-radio-release-links --env .env.stage # target a specific env
//
// The dry run reads inside a READ ONLY transaction, so the server refuses any
// write. The live run evaluates and writes inside one REPEATABLE READ
// transaction and changes a row only while its release_id is still the one
// evaluated.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

// removalSampleSize is how many removals the report lists without --verbose.
const removalSampleSize = 50

func main() {
	var (
		confirm bool
		verbose bool
		limit   int
		envFile string
	)
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print every removal and relink")
	flag.IntVar(&limit, "limit", 0, "Max plays scanned, lowest id first (0 = all)")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	loadEnv(envFile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Radio Release Links Backfill (%s) ===\n", mode)
	// The resolved target prints before connecting, so a mistargeted --confirm
	// is visible before any write.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n", os.Getenv(config.EnvEnvironment), redactDatabaseURL(cfg.Database.URL))

	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	if err := run(db.GetDB(), confirm, limit, verbose, os.Stdout); err != nil {
		log.Fatalf("backfill: %v", err)
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

// run evaluates (and with confirm, applies) the backfill and prints the report.
func run(database *gorm.DB, confirm bool, limit int, verbose bool, out io.Writer) error {
	report, err := catalog.BackfillRadioReleaseLinks(database, catalog.RadioReleaseLinkBackfillOptions{
		DryRun: !confirm,
		Limit:  limit,
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, formatReport(report, confirm, verbose))
	return err
}

var outcomeLabels = map[catalog.RadioReleaseLinkOutcome]string{
	catalog.ReleaseLinkKeptCredited:          "Kept: release credited to the play's artist",
	catalog.ReleaseLinkKeptUniqueTitle:       "Kept: no artist link, unique title, sole credit has the play's artist name",
	catalog.ReleaseLinkKeptAdminSet:          "Kept: set by an admin (link_radio_play), rule would change it",
	catalog.ReleaseLinkRemovedNotCredited:    "Removed: release not credited to the play's artist",
	catalog.ReleaseLinkRemovedUnlinkedArtist: "Removed: no artist link, title-only link (open question, default drop)",
	catalog.ReleaseLinkRelinkedCredited:      "Relinked: to the release with this title credited to the play's artist",
	catalog.ReleaseLinkRelinkedUniqueTitle:   "Relinked: no artist link, to the unique title whose sole credit matches",
}

func formatReport(r *catalog.RadioReleaseLinkReport, confirm, verbose bool) string {
	out := &strings.Builder{}
	fmt.Fprintln(out, "=== Summary ===")
	fmt.Fprintf(out, "Plays with a release link scanned: %d\n", r.Scanned)
	for _, outcome := range catalog.RadioReleaseLinkOutcomes {
		fmt.Fprintf(out, "  %-78s %d\n", outcomeLabels[outcome]+":", r.Counts[outcome])
	}
	fmt.Fprintln(out)

	if verbose {
		fmt.Fprintf(out, "--- Every removal and relink (%d) ---\n", len(r.Changes))
		for _, c := range r.Changes {
			fmt.Fprintln(out, formatChange(c))
		}
	} else {
		var removals []catalog.RadioReleaseLinkChange
		for _, c := range r.Changes {
			if c.Outcome.IsRemoval() {
				removals = append(removals, c)
			}
		}
		sample := sampleRemovals(removals, removalSampleSize)
		fmt.Fprintf(out, "--- Sample of removals (%d of %d) ---\n", len(sample), len(removals))
		for _, c := range sample {
			fmt.Fprintln(out, formatChange(c))
		}
	}
	fmt.Fprintln(out)

	if confirm {
		fmt.Fprintf(out, "LIVE: release links cleared %d, relinked %d.\n", r.Cleared, r.Relinked)
	} else {
		fmt.Fprintln(out, "DRY RUN: no DB writes. Re-run with --confirm to apply.")
	}
	return out.String()
}

func formatChange(c catalog.RadioReleaseLinkChange) string {
	artist := "none"
	if c.ArtistID != nil {
		artist = fmt.Sprintf("%d", *c.ArtistID)
	}
	credits := "(none)"
	if len(c.CurrentReleaseArtists) > 0 {
		credits = strings.Join(c.CurrentReleaseArtists, "; ")
	}
	line := fmt.Sprintf("  play %d [%s] artist %q (artist_id %s) album %q: release %d %q credited to %s",
		c.PlayID, c.Outcome, c.ArtistName, artist, c.AlbumTitle, c.CurrentReleaseID, c.CurrentReleaseTitle, credits)
	if c.NewReleaseID != nil {
		line += fmt.Sprintf(" -> release %d %q", *c.NewReleaseID, c.NewReleaseTitle)
	}
	return line
}

// sampleRemovals picks up to n removals, split evenly between the two removal
// outcomes (a smaller group cedes its unused share to the other) and spread
// evenly across each group's play-id order.
func sampleRemovals(removals []catalog.RadioReleaseLinkChange, n int) []catalog.RadioReleaseLinkChange {
	var notCredited, unlinked []catalog.RadioReleaseLinkChange
	for _, c := range removals {
		if c.Outcome == catalog.ReleaseLinkRemovedNotCredited {
			notCredited = append(notCredited, c)
		} else {
			unlinked = append(unlinked, c)
		}
	}
	shareA := min(len(notCredited), n/2)
	shareB := min(len(unlinked), n-shareA)
	shareA = min(len(notCredited), n-shareB)
	return append(spread(notCredited, shareA), spread(unlinked, shareB)...)
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
