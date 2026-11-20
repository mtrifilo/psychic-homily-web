package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

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
// is left for planRows to refuse, so one bad row never hides the others.
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

// runWorklist plans every row against the database as it stands, then, when
// confirm is set, applies the rows the plan accepted, each in its own
// transaction and in file order.
//
// Every verdict is reached before the first write, and the file-level checks in
// refuseEntangledRows keep the rows independent of each other, so the dry run
// and the confirmed run accept exactly the same rows.
func runWorklist(db *gorm.DB, rows []worklistRow, confirm bool, out io.Writer) runSummary {
	plans := make([]*rowPlan, len(rows))
	for i, row := range rows {
		plans[i] = &rowPlan{Row: row}
	}
	refuseMalformedRows(plans)
	refuseEntangledRows(plans)

	venues := catalog.NewVenueService(db).WithAddressGeocoder(nil)
	for _, p := range plans {
		if p.Status == "" {
			planRow(db, venues, p)
		}
	}
	refuseCollidingRenames(plans)

	summary := runSummary{}
	for _, p := range plans {
		if confirm && p.Status == statusWouldApply {
			applyRow(db, venues, p)
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
		case r.Action == actionKeep && r.Target != "":
			p.refuse("keep takes no target, got %q", r.Target)
		case r.Action != actionMerge && r.Action != actionRename && r.Action != actionKeep:
			p.refuse("unknown action %q (want merge, rename or keep)", r.Action)
		}
	}
}

// refuseEntangledRows refuses rows whose outcome would depend on another row of
// the same file: a room named on two rows, and a merge into a room that another
// row merges away. Both sides of each conflict are refused, so neither is
// applied by the accident of file order. A merge already refused merges nothing
// away, so it entangles no other row.
func refuseEntangledRows(plans []*rowPlan) {
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

	mergedAwayOn := map[string]*rowPlan{}
	for _, p := range plans {
		if p.Status == "" && p.Row.Action == actionMerge {
			mergedAwayOn[p.Row.RoomSlug] = p
		}
	}
	for _, p := range plans {
		if p.Status != "" || p.Row.Action != actionMerge {
			continue
		}
		other, ok := mergedAwayOn[p.Row.Target]
		if !ok {
			continue
		}
		p.refuse("target %s is itself merged away on line %d; merge both rooms into the final room instead", p.Row.Target, other.Row.Line)
		other.refuse("room %s is the merge target on line %d; merge both rooms into the final room instead", other.Row.RoomSlug, p.Row.Line)
	}
}

// refuseCollidingRenames refuses renames that would give two rooms of one place
// the same name, which the per-row check cannot see because neither name exists
// yet.
func refuseCollidingRenames(plans []*rowPlan) {
	byKey := map[string][]*rowPlan{}
	for _, p := range plans {
		if p.Status != statusWouldApply || p.Row.Action != actionRename {
			continue
		}
		key := placeKey(p.Room) + "|" + utils.NormalizeVenueName(p.Row.Target)
		byKey[key] = append(byKey[key], p)
	}
	for _, group := range byKey {
		if len(group) < 2 {
			continue
		}
		lines := make([]int, len(group))
		for i, p := range group {
			lines[i] = p.Row.Line
		}
		for _, p := range group {
			p.refuse("renames on lines %s would give two rooms in %s, %s the same name", joinInts(lines), p.Room.City, p.Room.State)
		}
	}
}

// planRow loads the rooms a row names and reaches its verdict without writing.
func planRow(db *gorm.DB, venues *catalog.VenueService, p *rowPlan) {
	room, err := venueBySlug(db, p.Row.RoomSlug)
	if err != nil {
		p.refuse("load room %s: %v", p.Row.RoomSlug, err)
		return
	}
	p.Room = room

	switch p.Row.Action {
	case actionMerge:
		planMerge(db, venues, p)
	case actionRename:
		planRename(db, p)
	case actionKeep:
		if room == nil {
			p.refuse("no room has slug %s", p.Row.RoomSlug)
			return
		}
		p.Status = statusKept
		p.Detail = []string{describeRoom("room", db, room)}
	}
}

func planMerge(db *gorm.DB, venues *catalog.VenueService, p *rowPlan) {
	into, err := venueBySlug(db, p.Row.Target)
	if err != nil {
		p.refuse("load target %s: %v", p.Row.Target, err)
		return
	}
	p.Into = into

	if p.Room == nil {
		if into != nil && alreadyMerged(db, p.Row.RoomSlug, into.ID) {
			p.Status = statusDone
			p.Detail = []string{
				fmt.Sprintf("an audit record shows %s was merged into %s", p.Row.RoomSlug, p.Row.Target),
				describeRoom("into", db, into),
			}
			return
		}
		p.refuse("no room has slug %s", p.Row.RoomSlug)
		return
	}
	if into == nil {
		p.refuse("no room has target slug %s", p.Row.Target)
		return
	}
	if into.ID == p.Room.ID {
		p.refuse("target %s is the room itself", p.Row.Target)
		return
	}

	p.Detail = []string{describeRoom("from", db, p.Room), describeRoom("into", db, into)}
	if placeKey(p.Room) != placeKey(into) {
		p.refuse("the rooms are in different places (%s, %s and %s, %s)", p.Room.City, p.Room.State, into.City, into.State)
		return
	}

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

func planRename(db *gorm.DB, p *rowPlan) {
	if p.Room == nil {
		p.refuse("no room has slug %s", p.Row.RoomSlug)
		return
	}
	p.Detail = []string{describeRoom("room", db, p.Room)}
	if p.Room.Name == p.Row.Target {
		p.Status = statusDone
		p.Detail = append(p.Detail, fmt.Sprintf("already named %q", p.Row.Target))
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := renameInTx(tx, p.Room.ID, p.Row.Target); err != nil {
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

// renameInTx renames one room inside tx through UpdateVenue, after locking the
// room and refusing a name that another room in the same city and state
// already carries under PSY-1596's normalization. UpdateVenue's own check adds
// the exact, case-insensitive match across every room of that city name. The
// slug is left exactly as UpdateVenue leaves it.
func renameInTx(tx *gorm.DB, roomID uint, name string) error {
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
		if placeKey(other) == placeKey(&room) && utils.NormalizeVenueName(other.Name) == want {
			return fmt.Errorf("%s, %s already has %q (slug %s); merge the rooms instead", room.City, room.State, other.Name, slugOf(other))
		}
	}

	venues := catalog.NewVenueService(tx).WithAddressGeocoder(nil)
	if _, err := venues.UpdateVenue(room.ID, &contracts.UpdateVenueRequest{Name: &name}); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// applyRow applies one accepted row in its own transaction. A failure leaves
// that row unapplied and is reported as a refusal.
func applyRow(db *gorm.DB, venues *catalog.VenueService, p *rowPlan) {
	switch p.Row.Action {
	case actionMerge:
		result, err := venues.MergeVenuesInSamePlace(p.Into.ID, p.Room.ID, 0)
		if err != nil {
			p.refuse("merge failed, nothing changed: %v", err)
			return
		}
		p.Detail = append(p.Detail, "merged: "+describeMergeResult(result))
	case actionRename:
		err := db.Transaction(func(tx *gorm.DB) error {
			return renameInTx(tx, p.Room.ID, p.Row.Target)
		})
		if err != nil {
			p.refuse("rename failed, nothing changed: %v", err)
			return
		}
	}
	p.Status = statusApplied
}

// alreadyMerged reports whether the merge audit trail records a room with this
// slug merged into the room with id into. It is the only record of a merged
// room's slug once its row is gone.
func alreadyMerged(db *gorm.DB, slug string, into uint) bool {
	var found bool
	err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM audit_logs
		 WHERE action = ? AND entity_type = 'venue' AND entity_id = ?
		   AND metadata->>'merged_venue_slug' = ?)`,
		catalog.AuditActionMergeVenues, into, slug).Scan(&found).Error
	return err == nil && found
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

// placeKey is a room's city and state as the merge and rename checks compare
// them: trimmed and case-insensitive.
func placeKey(v *catalogm.Venue) string {
	return strings.ToLower(strings.TrimSpace(v.City)) + "|" + strings.ToLower(strings.TrimSpace(v.State))
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
	fmt.Fprintf(out, "[line %d] %s %s%s\n", p.Row.Line, p.Row.Action, p.Row.RoomSlug, target)
	for _, d := range p.Detail {
		fmt.Fprintf(out, "  %s\n", d)
	}
	if p.Row.Note != "" {
		fmt.Fprintf(out, "  note: %s\n", p.Row.Note)
	}
	if p.Reason != "" {
		fmt.Fprintf(out, "  %s: %s\n\n", p.Status, p.Reason)
		return
	}
	fmt.Fprintf(out, "  %s\n\n", p.Status)
}

func printSummary(out io.Writer, s runSummary, confirm bool) {
	fmt.Fprintln(out, "=== Summary ===")
	statuses := []rowStatus{statusApplied, statusWouldApply, statusDone, statusKept, statusRefused}
	for _, st := range statuses {
		fmt.Fprintf(out, "%-13s %d\n", string(st)+":", s[st])
	}
	if !confirm {
		fmt.Fprintln(out, "DRY RUN: nothing was written. Re-run with --confirm to apply the rows marked WOULD APPLY.")
	}
	if s.Refused() {
		fmt.Fprintln(out, "Some rows were REFUSED; nothing was written for them. Exit status 1.")
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
