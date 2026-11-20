package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
	"psychic-homily-backend/internal/utils"
)

// requireAdminActor refuses an actor id that is not an active admin. Every
// write the tool makes is recorded against this user: the merge audit row and
// each rename's revision.
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

// The three actions a reviewed worklist row may carry.
const (
	actionMerge  = "merge"
	actionRename = "rename"
	actionKeep   = "keep"
)

// The columns a worklist must have. Their names are deliberately not the
// suggestion file's (suggested_action, suggested_target): a list of suggestions
// cannot be applied until a person has reviewed it and renamed those columns.
const (
	colRoomSlug = "room_slug"
	colAction   = "action"
	colTarget   = "target"
	colNote     = "note"
)

// worklistRow is one reviewed decision about one room.
type worklistRow struct {
	Line     int
	RoomSlug string
	Action   string
	Target   string
	Note     string
}

// parseWorklist reads a reviewed CSV. The header must name room_slug, action
// and target; note is optional and any other column is ignored, so the review
// context can stay in the file. Values are trimmed and the action is
// lowercased. A malformed file is an error for the whole file; a malformed row
// is left for runWorklist to refuse, so one bad row never hides the others.
func parseWorklist(r io.Reader) ([]worklistRow, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\ufeff"))

	reader := csv.NewReader(bytes.NewReader(raw))
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("worklist is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(name))
		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("header names column %q twice", name)
		}
		index[name] = i
	}
	for _, required := range []string{colRoomSlug, colAction, colTarget} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("header has no %q column (got %s); a suggestions file must be reviewed "+
				"and its suggested_action and suggested_target columns renamed to action and target before it can be applied",
				required, strings.Join(header, ","))
		}
	}

	field := func(record []string, name string) string {
		i, ok := index[name]
		if !ok {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	var rows []worklistRow
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read worklist: %w", err)
		}
		line, _ := reader.FieldPos(0)
		row := worklistRow{
			Line:     line,
			RoomSlug: field(record, colRoomSlug),
			Action:   strings.ToLower(field(record, colAction)),
			Target:   field(record, colTarget),
			Note:     field(record, colNote),
		}
		if row == (worklistRow{Line: line}) {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("worklist has a header and no rows")
	}
	return rows, nil
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

// rowPlan is a row together with the rooms it names and the verdict reached on
// the database as it stood before anything was applied.
type rowPlan struct {
	Row    worklistRow
	Room   *catalogm.Venue
	Into   *catalogm.Venue
	Status rowStatus
	Reason string
	Detail []string
}

func (p *rowPlan) refuse(format string, args ...any) {
	p.Status = statusRefused
	p.Reason = fmt.Sprintf(format, args...)
}

// runSummary counts rows by final status.
type runSummary map[rowStatus]int

// Refused reports whether any row was refused, which makes the run exit
// non-zero.
func (s runSummary) Refused() bool { return s[statusRefused] > 0 }

// runReport is what a run did: the counts, and every merge it applied, which
// the caller checks against the audit trail once the audit writes have drained.
type runReport struct {
	Summary runSummary
	Merged  []*rowPlan
}

// runWorklist plans every row against the database as it stands, then, when
// confirm is set, applies the rows the plan accepted, each in its own
// transaction and in file order. actorID is the admin the writes are recorded
// against.
//
// Every verdict is reached before the first write. The file-level checks
// (refuseRepeatedRooms, refuseEntangledMerges, refuseCollidingRenames) refuse
// every pair of rows whose verdict would depend on the other, so the dry run
// accepts the same rows the confirmed run applies. A write can still fail if
// the database changes between the plan and the write; that row is reported as
// refused. A merge preview's counts can differ from its write's when one show is
// booked at rooms of two different merges, which is why a confirmed merge
// prints the counts it actually applied.
func runWorklist(db *gorm.DB, rows []worklistRow, actorID uint, confirm bool, out io.Writer) runReport {
	plans := make([]*rowPlan, len(rows))
	for i, row := range rows {
		plans[i] = &rowPlan{Row: row}
	}
	refuseMalformedRows(plans)
	refuseRepeatedRooms(plans)

	venues := catalog.NewVenueService(db).WithAddressGeocoder(nil)
	for _, p := range plans {
		if p.Status == "" {
			planRow(db, venues, actorID, p)
		}
	}
	refuseEntangledMerges(plans)
	refuseCollidingRenames(plans)

	report := runReport{Summary: runSummary{}}
	for _, p := range plans {
		if confirm && p.Status == statusWouldApply {
			applyRow(db, venues, actorID, p)
			if p.Status == statusApplied && p.Row.Action == actionMerge {
				report.Merged = append(report.Merged, p)
			}
		}
		printPlan(out, p)
		report.Summary[p.Status]++
	}
	printSummary(out, report.Summary, confirm)
	return report
}

// reportUnrecordedMerges prints every merge in report whose audit row is
// missing. The audit row is written after the merge commits, on a background
// writer that may drop it; without it a later run cannot tell the merge
// happened and refuses the row as an unknown slug. It returns how many were
// missing.
func reportUnrecordedMerges(db *gorm.DB, report runReport, out io.Writer) int {
	venues := catalog.NewVenueService(db).WithAddressGeocoder(nil)
	missing := 0
	for _, p := range report.Merged {
		recorded, err := venues.WasMergedInto(p.Row.RoomSlug, p.Into.ID)
		if err == nil && recorded {
			continue
		}
		missing++
		reason := "no audit row"
		if err != nil {
			reason = err.Error()
		}
		printf(out, "WARNING: merge of %s into %s (line %d) is applied but has %s; a later run will refuse that row as an unknown slug\n",
			p.Row.RoomSlug, p.Row.Target, p.Row.Line, reason)
	}
	return missing
}

// refuseMalformedRows refuses rows whose fields cannot describe a decision.
func refuseMalformedRows(plans []*rowPlan) {
	for _, p := range plans {
		r := p.Row
		switch {
		case r.RoomSlug == "":
			p.refuse("room_slug is empty")
		case r.Action == actionMerge && r.Target == "":
			p.refuse("merge needs a target: the slug of the room to keep")
		case r.Action == actionMerge && r.Target == r.RoomSlug:
			p.refuse("target %s is the room itself", r.Target)
		case r.Action == actionRename && r.Target == "":
			p.refuse("rename needs a target: the new name")
		case r.Action == actionRename && utils.NormalizeVenueName(r.Target) == "":
			p.refuse("rename target %q has no letters or digits", r.Target)
		case r.Action == actionRename && slugShaped.MatchString(r.Target):
			p.refuse("rename target %q looks like a slug, not a name; a merge takes a slug", r.Target)
		case r.Action == actionKeep && r.Target != "":
			p.refuse("keep takes no target, got %q", r.Target)
		case r.Action != actionMerge && r.Action != actionRename && r.Action != actionKeep:
			p.refuse("unknown action %q (want merge, rename or keep)", r.Action)
		}
	}
}

// slugShaped matches a lowercase, hyphen-joined string with no spaces: the shape
// of a slug, which a venue's display name never has.
var slugShaped = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// refuseRepeatedRooms refuses every row of a room named on more than one row:
// which decision wins would otherwise depend on file order.
func refuseRepeatedRooms(plans []*rowPlan) {
	linesBySlug := map[string][]int{}
	for _, p := range plans {
		if p.Row.RoomSlug != "" {
			linesBySlug[p.Row.RoomSlug] = append(linesBySlug[p.Row.RoomSlug], p.Row.Line)
		}
	}
	for _, p := range plans {
		if lines := linesBySlug[p.Row.RoomSlug]; len(lines) > 1 {
			p.refuse("room %s appears on %d rows (lines %s); decide it once", p.Row.RoomSlug, len(lines), joinInts(lines))
		}
	}
}

// refuseEntangledMerges refuses merges whose write would depend on another
// merge of the same run: a merge into a room that another row merges away, and
// two merges into one room (the second merge's preview cannot see the shows the
// first one moves, so its duplicate count would be wrong). Every side of each
// conflict is refused, so none is applied by the accident of file order.
//
// It runs after planning and counts only merges that would apply: a merge that
// is already done or refused moves nothing in this run, so it entangles no
// other row, and a file whose first merges were applied by an earlier run can
// go on to apply the rest.
func refuseEntangledMerges(plans []*rowPlan) {
	pending := func(p *rowPlan) bool { return p.Status == statusWouldApply && p.Row.Action == actionMerge }

	mergedAwayOn := map[string]*rowPlan{}
	for _, p := range plans {
		if pending(p) {
			mergedAwayOn[p.Row.RoomSlug] = p
		}
	}
	mergesInto := map[string][]*rowPlan{}
	for _, p := range plans {
		if pending(p) {
			mergesInto[p.Row.Target] = append(mergesInto[p.Row.Target], p)
		}
	}

	for _, p := range plans {
		if !pending(p) {
			continue
		}
		other, ok := mergedAwayOn[p.Row.Target]
		if !ok {
			continue
		}
		p.refuse("target %s is itself merged away on line %d; merge both rooms into the final room instead", p.Row.Target, other.Row.Line)
		other.refuse("room %s is the merge target on line %d; merge both rooms into the final room instead", other.Row.RoomSlug, p.Row.Line)
	}
	refuseGroups(mergesInto, func(lines string, p *rowPlan) {
		p.refuse("target %s receives merges on lines %s; apply one of them per run so each preview is exact", p.Row.Target, lines)
	})
}

// refuseCollidingRenames refuses renames that would give two rooms the same
// name where the per-row check cannot see it, because neither name exists yet.
// Two keys, because two rules forbid it: the same normalized name in one city
// and state (the tool's own rule), and the same lowercase name in one city name
// in any state (UpdateVenue's check and the unique index on
// (lower(name), lower(city))).
func refuseCollidingRenames(plans []*rowPlan) {
	samePlace := map[string][]*rowPlan{}
	sameCityName := map[string][]*rowPlan{}
	for _, p := range plans {
		if p.Status != statusWouldApply || p.Row.Action != actionRename {
			continue
		}
		placeKey := catalog.VenuePlaceKey(p.Room) + "|" + utils.NormalizeVenueName(p.Row.Target)
		samePlace[placeKey] = append(samePlace[placeKey], p)
		indexKey := strings.ToLower(p.Room.City) + "|" + strings.ToLower(p.Row.Target)
		sameCityName[indexKey] = append(sameCityName[indexKey], p)
	}
	refuse := func(lines string, p *rowPlan) {
		p.refuse("renames on lines %s would give two rooms in %s the same name", lines, p.Room.City)
	}
	refuseGroups(samePlace, refuse)
	refuseGroups(sameCityName, refuse)
}

// refuseGroups calls refuse on every row of every group holding two or more
// rows, with the group's line numbers.
func refuseGroups(groups map[string][]*rowPlan, refuse func(lines string, p *rowPlan)) {
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		lines := make([]int, len(group))
		for i, p := range group {
			lines[i] = p.Row.Line
		}
		for _, p := range group {
			refuse(joinInts(lines), p)
		}
	}
}

// planRow loads the rooms a row names and reaches its verdict without writing.
func planRow(db *gorm.DB, venues *catalog.VenueService, actorID uint, p *rowPlan) {
	room, err := venueBySlug(db, p.Row.RoomSlug)
	if err != nil {
		p.refuse("load room %s: %v", p.Row.RoomSlug, err)
		return
	}
	p.Room = room

	if p.Row.Action == actionMerge {
		planMerge(db, venues, p)
		return
	}
	if room == nil {
		p.refuse("no room has slug %s", p.Row.RoomSlug)
		return
	}
	p.Detail = []string{describeRoom("room", db, room)}
	if p.Row.Action == actionRename {
		planRename(db, actorID, p)
		return
	}
	p.Status = statusKept
}

// planMerge previews a merge. A missing room is not refused outright: the merge
// audit trail may show this very row was applied by an earlier run.
func planMerge(db *gorm.DB, venues *catalog.VenueService, p *rowPlan) {
	into, err := venueBySlug(db, p.Row.Target)
	if err != nil {
		p.refuse("load target %s: %v", p.Row.Target, err)
		return
	}
	p.Into = into

	if p.Room == nil {
		if into == nil {
			p.refuse("no room has slug %s", p.Row.RoomSlug)
			return
		}
		merged, err := venues.WasMergedInto(p.Row.RoomSlug, into.ID)
		if err != nil {
			p.refuse("%v", err)
			return
		}
		if !merged {
			p.refuse("no room has slug %s", p.Row.RoomSlug)
			return
		}
		p.Status = statusDone
		p.Detail = []string{
			fmt.Sprintf("an audit record shows %s was merged into %s", p.Row.RoomSlug, p.Row.Target),
			describeRoom("into", db, into),
		}
		return
	}
	if into == nil {
		p.refuse("no room has target slug %s", p.Row.Target)
		return
	}

	p.Detail = []string{describeRoom("from", db, p.Room), describeRoom("into", db, into)}
	preview, err := venues.PreviewMergeVenuesInSamePlace(into.ID, p.Room.ID)
	if err != nil {
		p.refuse("merge preview: %v", err)
		return
	}
	p.Status = statusWouldApply
	p.Detail = append(p.Detail,
		"preview: "+describeMergeResult(preview),
		fmt.Sprintf("after the merge /venues/%s answers 404; /venues/%s keeps its slug", p.Row.RoomSlug, p.Row.Target))
}

func planRename(db *gorm.DB, actorID uint, p *rowPlan) {
	if p.Room.Name == p.Row.Target {
		p.Status = statusDone
		p.Detail = append(p.Detail, fmt.Sprintf("already named %q", p.Row.Target))
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := renameInTx(tx, actorID, p.Room.ID, p.Row.Target); err != nil {
			return err
		}
		return errDryRunRollback
	})
	if !errors.Is(err, errDryRunRollback) {
		p.refuse("%v", err)
		return
	}
	p.Status = statusWouldApply
	p.Detail = append(p.Detail,
		fmt.Sprintf("name: %q -> %q", p.Room.Name, p.Row.Target),
		fmt.Sprintf("slug: %s (unchanged)", slugOf(p.Room)))
}

// errDryRunRollback ends a dry-run rename's transaction after the real update
// has run, so the dry run reports exactly what the update would do.
var errDryRunRollback = errors.New("dry run rollback")

// renameRevisionSummary is the summary on the revision a rename records.
const renameRevisionSummary = "Renamed from a reviewed venue worklist"

// renameInTx renames one room inside tx through UpdateVenue and records the
// change as a revision attributed to actorID, the same record an admin rename
// through the API leaves. Before writing it locks the room and refuses a name
// another room of the same city and state already carries under
// NormalizeVenueName; UpdateVenue's own check adds the exact, case-insensitive
// match across every room of that city name. The slug is left exactly as
// UpdateVenue leaves it.
func renameInTx(tx *gorm.DB, actorID, roomID uint, name string) error {
	var room catalogm.Venue
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&room, roomID).Error; err != nil {
		return fmt.Errorf("lock room %d: %w", roomID, err)
	}

	var sameCity []catalogm.Venue
	if err := tx.Where("id <> ? AND LOWER(TRIM(city)) = LOWER(TRIM(?))", room.ID, room.City).
		Order("id").Find(&sameCity).Error; err != nil {
		return fmt.Errorf("load rooms in %s: %w", room.City, err)
	}
	want := utils.NormalizeVenueName(name)
	for i := range sameCity {
		other := &sameCity[i]
		if catalog.VenuePlaceKey(other) == catalog.VenuePlaceKey(&room) && utils.NormalizeVenueName(other.Name) == want {
			return fmt.Errorf("%s, %s already has %q (slug %s); merge the rooms instead", room.City, room.State, other.Name, slugOf(other))
		}
	}

	venues := catalog.NewVenueService(tx).WithAddressGeocoder(nil)
	before, err := venues.GetVenue(room.ID)
	if err != nil {
		return fmt.Errorf("read room %d: %w", room.ID, err)
	}
	after, err := venues.UpdateVenue(room.ID, &contracts.UpdateVenueRequest{Name: &name})
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	changes := revisiondiff.Compare(before, after, revisiondiff.VenueFields)
	if err := admin.NewRevisionService(tx).RecordRevision("venue", room.ID, actorID, changes, renameRevisionSummary); err != nil {
		return fmt.Errorf("record rename revision: %w", err)
	}
	return nil
}

// applyRow applies one accepted row in its own transaction. A failure leaves
// that row unapplied and is reported as a refusal.
func applyRow(db *gorm.DB, venues *catalog.VenueService, actorID uint, p *rowPlan) {
	switch p.Row.Action {
	case actionMerge:
		result, err := venues.MergeVenuesInSamePlace(p.Into.ID, p.Room.ID, actorID)
		if err != nil {
			// An error at COMMIT does not prove the commit failed. The merged room's
			// row is deleted only by a committed merge, so its presence decides.
			gone, lookupErr := venueGone(db, p.Room.ID)
			if lookupErr != nil || !gone {
				p.refuse("merge failed, room %s is still there: %v", p.Row.RoomSlug, err)
				return
			}
			p.Detail = append(p.Detail, fmt.Sprintf("merge reported an error but room %s is gone, so it committed: %v", p.Row.RoomSlug, err))
			p.Status = statusApplied
			return
		}
		p.Detail = append(p.Detail, "merged: "+describeMergeResult(result))
	case actionRename:
		err := db.Transaction(func(tx *gorm.DB) error {
			return renameInTx(tx, actorID, p.Room.ID, p.Row.Target)
		})
		if err != nil {
			p.refuse("rename failed, nothing changed: %v", err)
			return
		}
	}
	p.Status = statusApplied
}

// venueGone reports whether no room has this id.
func venueGone(db *gorm.DB, id uint) (bool, error) {
	var n int64
	if err := db.Model(&catalogm.Venue{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return false, err
	}
	return n == 0, nil
}

// venueBySlug returns the room with this slug, or nil when none has it.
func venueBySlug(db *gorm.DB, slug string) (*catalogm.Venue, error) {
	var v catalogm.Venue
	err := db.Where("slug = ?", slug).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func slugOf(v *catalogm.Venue) string {
	if v.Slug == nil {
		return "(none)"
	}
	return *v.Slug
}

// describeRoom is one line naming a room the way a reviewer checks it: name,
// place, id, slug and how many shows it holds.
func describeRoom(label string, db *gorm.DB, v *catalogm.Venue) string {
	var shows int64
	countText := "?"
	if err := db.Table("show_venues").Where("venue_id = ?", v.ID).Count(&shows).Error; err == nil {
		countText = fmt.Sprintf("%d", shows)
	}
	return fmt.Sprintf("%s: %q (%s, %s) id=%d slug=%s shows=%s", label, v.Name, v.City, v.State, v.ID, slugOf(v), countText)
}

func describeMergeResult(r *contracts.MergeVenueResult) string {
	return fmt.Sprintf("duplicate_shows=%d support_acts_rescued=%d show_venues_moved=%d show_artists_moved=%d "+
		"festival_venues_moved=%d festival_artists_moved=%d confirmations_moved=%d filters_updated=%d entity_refs_moved=%d",
		r.DuplicateShows, r.SupportActsRescued, r.ShowVenuesMoved, r.ShowArtistsMoved,
		r.FestivalVenuesMoved, r.FestivalArtistsMoved, r.ConfirmationsMoved, r.FiltersUpdated, r.EntityRefsMoved)
}

func printPlan(out io.Writer, p *rowPlan) {
	target := ""
	if p.Row.Target != "" {
		target = " -> " + p.Row.Target
	}
	printf(out, "[line %d] %s %s%s\n", p.Row.Line, p.Row.Action, p.Row.RoomSlug, target)
	for _, d := range p.Detail {
		printf(out, "  %s\n", d)
	}
	if p.Row.Note != "" {
		printf(out, "  note: %s\n", p.Row.Note)
	}
	if p.Reason != "" {
		printf(out, "  %s: %s\n\n", p.Status, p.Reason)
		return
	}
	printf(out, "  %s\n\n", p.Status)
}

// printf writes one piece of the report. The report goes to a terminal or a
// pipe, and a failed write there has nowhere better to be reported.
func printf(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format, args...)
}

func printSummary(out io.Writer, s runSummary, confirm bool) {
	printf(out, "=== Summary ===\n")
	statuses := []rowStatus{statusApplied, statusWouldApply, statusDone, statusKept, statusRefused}
	for _, st := range statuses {
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
