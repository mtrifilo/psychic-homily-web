package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// requireAdminActor refuses an actor id that is not an active admin. Every
// write the tool makes is recorded against this user.
func requireAdminActor(db *gorm.DB, id uint) error {
	if id == 0 {
		return fmt.Errorf("--actor is required: the id of the admin user the writes are recorded against")
	}
	var u authm.User
	err := db.First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("--actor %d: no such user", id)
	}
	if err != nil {
		return fmt.Errorf("--actor %d: %w", id, err)
	}
	if !u.IsAdmin || !u.IsActive {
		return fmt.Errorf("--actor %d: not an active admin", id)
	}
	return nil
}

// rowStatus is what happened to, or would happen to, one row.
type rowStatus string

const (
	statusWouldApply rowStatus = "WOULD APPLY"
	statusApplied    rowStatus = "APPLIED"
	statusDone       rowStatus = "ALREADY DONE"
	statusKept       rowStatus = "KEPT"
	statusRefused    rowStatus = "REFUSED"
)

// showRef names one show a row rewrites the bill of.
type showRef struct {
	ID   uint
	Slug string
}

// rowPlan is a row, the verdict reached on the database as it stood before
// anything was applied, and the footprint the file-level checks compare: which
// artists, names and bills the row's write would touch.
type rowPlan struct {
	Row     worklistRow
	Status  rowStatus
	Reasons []string
	Detail  []string

	SubjectID   uint      // the row's artist
	TargetID    uint      // a merge's target, also set on a merge already done
	BillShows   []showRef // shows whose bill a remove or split rewrites
	FoundActIDs []uint    // existing artists a split puts on a bill
	NewNames    []string  // lowercased names a rename gives or a split creates
}

func (p *rowPlan) refuse(format string, args ...any) {
	p.Status = statusRefused
	p.Reasons = append(p.Reasons, fmt.Sprintf(format, args...))
}

// runSummary counts rows by final status.
type runSummary map[rowStatus]int

// Refused reports whether any row was refused, which makes the run exit
// non-zero.
func (s runSummary) Refused() bool { return s[statusRefused] > 0 }

// runWorklist plans every row against the database as it stands, then, when
// confirm is set, applies the rows the plan accepted, each in its own
// transaction and in file order. actorID is the admin the writes are recorded
// against.
//
// Every verdict is reached before the first write: each row is planned by
// running its write in a transaction that is rolled back, against the
// database as it stood before the run. The file-level checks then refuse every
// row whose write would depend on another row's, decided over the whole file
// before any of them is refused, so the set of rows applied does not depend on
// the order of the file. A write can still fail if the database changes
// between the plan and the write; that row is reported as refused.
func runWorklist(db *gorm.DB, rows []worklistRow, actorID uint, confirm bool, out io.Writer) runSummary {
	plans := make([]*rowPlan, len(rows))
	for i, row := range rows {
		plans[i] = &rowPlan{Row: row}
	}
	refuseMalformedRows(plans)
	refuseRepeatedArtists(plans)

	t := &triage{db: db, actor: actorID}
	for _, p := range plans {
		if p.Status == "" {
			t.plan(p)
		}
	}
	refuseEntangledRows(plans)

	summary := runSummary{}
	for _, p := range plans {
		if confirm && p.Status == statusWouldApply {
			t.apply(p)
		}
		printPlan(out, p)
		summary[p.Status]++
	}
	printSummary(out, summary, confirm)
	return summary
}

// refuseMalformedRows refuses rows whose fields cannot describe a decision.
func refuseMalformedRows(plans []*rowPlan) {
	for _, p := range plans {
		r := p.Row
		switch {
		case r.ArtistSlug == "":
			p.refuse("artist_slug is empty")
		case r.ShowSlug != "" && r.Action != actionRemove && r.Action != actionSplit:
			p.refuse("show_slug applies to remove and split only; clear it for a %s", r.Action)
		}
		if p.Status != "" {
			continue
		}
		switch r.Action {
		case actionRename:
			switch {
			case r.Target == "":
				p.refuse("rename needs a target: the new name")
			case !hasLetterOrDigit(r.Target):
				p.refuse("rename target %q has no letters or digits", r.Target)
			case slugShaped.MatchString(r.Target):
				p.refuse("rename target %q looks like a slug, not a name; a merge takes a slug", r.Target)
			}
		case actionMerge:
			switch r.Target {
			case "":
				p.refuse("merge needs a target: the slug of the artist to keep")
			case r.ArtistSlug:
				p.refuse("target %s is the artist itself", r.Target)
			}
		case actionRemove:
			if r.Target != "" && !strings.EqualFold(r.Target, removeTargetTitle) {
				p.refuse("remove takes no target, or %q to write the name to the show's title; got %q", removeTargetTitle, r.Target)
			}
		case actionSplit:
			if r.Target == "" {
				p.refuse("split needs a target: the act names separated by %q", splitActSeparator)
			} else if _, err := splitActNames(r.Target, ""); err != nil {
				p.refuse("%v", err)
			}
		case actionKeep:
			if r.Target != "" {
				p.refuse("keep takes no target, got %q", r.Target)
			}
		default:
			p.refuse("unknown action %q (want rename, merge, remove, split or keep)", r.Action)
		}
	}
}

// refuseRepeatedArtists refuses every row of an artist named on more than one
// row: which decision wins would otherwise depend on file order.
func refuseRepeatedArtists(plans []*rowPlan) {
	linesBySlug := map[string][]int{}
	for _, p := range plans {
		if p.Row.ArtistSlug != "" {
			linesBySlug[p.Row.ArtistSlug] = append(linesBySlug[p.Row.ArtistSlug], p.Row.Line)
		}
	}
	for _, p := range plans {
		if lines := linesBySlug[p.Row.ArtistSlug]; len(lines) > 1 {
			p.refuse("artist %s appears on %d rows (lines %s); decide it once", p.Row.ArtistSlug, len(lines), joinInts(lines))
		}
	}
}

// refuseEntangledRows refuses the rows whose write would depend on another
// row's write in the same run:
//
//   - a merge into an artist another row renames, merges away, removes or
//     splits;
//   - two merges into one artist (the second merge's preview cannot see what
//     the first moves);
//   - a split that puts on a bill an existing artist another row changes;
//   - two rows that would give artists the same name (renames, and the new acts
//     a split creates);
//   - two rows that rewrite the bill of the same show;
//   - a row that changes an artist that already received a merge done by an
//     earlier run of this file, which would leave that merge's row with no
//     target and the file unable to run clean again.
//
// Every verdict is decided over the rows as planned, before any is refused, so
// a refusal never changes which other rows count and the outcome does not
// depend on file order. Only rows that would apply count, except the last rule,
// which reads the merges already done.
func refuseEntangledRows(plans []*rowPlan) {
	var pending []*rowPlan
	subjectRows := map[uint][]*rowPlan{}
	mergesInto := map[uint][]*rowPlan{}
	doneInto := map[uint]*rowPlan{}
	nameRows := map[string][]*rowPlan{}
	billRows := map[uint][]*rowPlan{}
	billSlug := map[uint]string{}
	for _, p := range plans {
		switch p.Status {
		case statusWouldApply:
			pending = append(pending, p)
			subjectRows[p.SubjectID] = append(subjectRows[p.SubjectID], p)
			if p.Row.Action == actionMerge {
				mergesInto[p.TargetID] = append(mergesInto[p.TargetID], p)
			}
			for _, name := range p.NewNames {
				nameRows[name] = append(nameRows[name], p)
			}
			for _, s := range p.BillShows {
				billRows[s.ID] = append(billRows[s.ID], p)
				billSlug[s.ID] = s.Slug
			}
		case statusDone:
			if p.Row.Action == actionMerge {
				doneInto[p.TargetID] = p
			}
		}
	}

	reasons := map[*rowPlan][]string{}
	add := func(p *rowPlan, format string, args ...any) {
		reasons[p] = append(reasons[p], fmt.Sprintf(format, args...))
	}
	for _, p := range pending {
		if prior := doneInto[p.SubjectID]; prior != nil {
			add(p, "artist %s already received the merge on line %d; changing it would leave that row with no target, "+
				"so change it from a file without line %d", p.Row.ArtistSlug, prior.Row.Line, prior.Row.Line)
		}
		switch p.Row.Action {
		case actionMerge:
			for _, other := range subjectRows[p.TargetID] {
				add(p, "target %s is also changed on line %d (%s); merge into the final artist, or run them separately",
					p.Row.Target, other.Row.Line, other.Row.Action)
				add(other, "artist %s receives the merge on line %d; run them separately", other.Row.ArtistSlug, p.Row.Line)
			}
			if into := mergesInto[p.TargetID]; len(into) > 1 {
				add(p, "target %s receives merges on lines %s; apply one of them per run so each preview is exact",
					p.Row.Target, joinInts(linesOf(into)))
			}
		case actionSplit:
			for _, id := range p.FoundActIDs {
				for _, other := range subjectRows[id] {
					add(p, "an act it puts on the bill is changed on line %d (%s %s); run them separately",
						other.Row.Line, other.Row.Action, other.Row.ArtistSlug)
					add(other, "artist %s is an act the split on line %d puts on a bill; run them separately",
						other.Row.ArtistSlug, p.Row.Line)
				}
			}
		}
	}
	for name, group := range nameRows {
		if len(group) > 1 {
			for _, p := range group {
				add(p, "lines %s would each give an artist the name %q; apply one of them per run", joinInts(linesOf(group)), name)
			}
		}
	}
	for id, group := range billRows {
		if len(group) > 1 {
			for _, p := range group {
				add(p, "lines %s each rewrite the bill of show %s; apply one of them per run", joinInts(linesOf(group)), billSlug[id])
			}
		}
	}

	for p, rs := range reasons {
		sort.Strings(rs)
		for i, r := range rs {
			if i > 0 && r == rs[i-1] {
				continue
			}
			p.refuse("%s", r)
		}
	}
}

func linesOf(plans []*rowPlan) []int {
	lines := make([]int, len(plans))
	for i, p := range plans {
		lines[i] = p.Row.Line
	}
	return lines
}

func printPlan(out io.Writer, p *rowPlan) {
	head := fmt.Sprintf("[line %d] %s %s", p.Row.Line, p.Row.Action, p.Row.ArtistSlug)
	if p.Row.Target != "" {
		head += " -> " + p.Row.Target
	}
	if p.Row.ShowSlug != "" {
		head += " (show " + p.Row.ShowSlug + ")"
	}
	printf(out, "%s\n", head)
	for _, d := range p.Detail {
		printf(out, "  %s\n", d)
	}
	if p.Row.Note != "" {
		printf(out, "  note: %s\n", p.Row.Note)
	}
	if len(p.Reasons) == 0 {
		printf(out, "  %s\n\n", p.Status)
		return
	}
	printf(out, "  %s: %s\n\n", p.Status, strings.Join(p.Reasons, "; "))
}

// printf writes one piece of the report. The report goes to a terminal or a
// pipe, and a failed write there has nowhere better to be reported.
func printf(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format, args...)
}

func printSummary(out io.Writer, s runSummary, confirm bool) {
	printf(out, "=== Summary ===\n")
	for _, st := range []rowStatus{statusApplied, statusWouldApply, statusDone, statusKept, statusRefused} {
		printf(out, "%-13s %d\n", string(st)+":", s[st])
	}
	if !confirm {
		printf(out, "DRY RUN: nothing was written. Re-run with --confirm to apply the rows marked WOULD APPLY.\n")
	}
	if s.Refused() {
		printf(out, "Some rows were REFUSED; nothing was written for them. Exit status 1.\n")
	}
}

func joinInts(ns []int) string {
	sorted := append([]int(nil), ns...)
	sort.Ints(sorted)
	parts := make([]string, len(sorted))
	for i, n := range sorted {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ", ")
}
