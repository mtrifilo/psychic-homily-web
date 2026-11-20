package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

// A year-replicated show is one event written once per year: the same rooms,
// the same headliner, the same venue-local month and day and the same ticket
// URL, in several venue-local years. This file finds those groups and removes
// every copy but one through DeleteShow, so each removal runs the same
// reference sweep an admin delete does.
//
// The planner is deliberately narrow. A group is only acted on when nothing
// about it is ambiguous; everything else is reported with the reason it was
// left alone, for a human to decide. In particular a group is acted on only
// when its copies carry one origin (source and submitter), one ticket URL, at
// least one copy past the far-future bound, and copies to delete that were
// written within replicaCreatedWindow of each other (or, when only one copy is
// to be deleted, within replicaCreatedWindow of the copy kept): the signature
// of one write that fanned a date out across years, as opposed to a show that
// really recurs or a show someone else submitted.

// replicaCreatedWindow is how close together the copies a group deletes must
// have been created.
const replicaCreatedWindow = 10 * time.Minute

// ReplicatedShowCopy is one approved show inside a replicated group, or one
// show dated past the bound that belongs to no group.
type ReplicatedShowCopy struct {
	ShowID uint
	Slug   string
	// LocalDate is the venue-local calendar date, YYYY-MM-DD, read in the
	// primary venue's zone exactly as shared.VenueLocalDateSQL reads it.
	LocalDate     string
	EventDate     time.Time
	CreatedAt     time.Time
	TicketURL     string
	VenueName     string
	HeadlinerName string
	Source        string
	// SubmittedBy is the submitting user's id, 0 when the column is NULL.
	SubmittedBy uint
	// Status is the show's status. Copies and Beyond rows are always
	// approved; a group's Siblings never are.
	Status string

	// Refs counts the rows that name this show, per table: the polymorphic
	// inventory DeleteShow sweeps, the alert rows beside it, and the foreign
	// keys a delete cascades or nulls. Only non-zero tables appear.
	Refs []ReplicaRefCount

	venueIDs    string
	headlinerID uint
	localToday  string
}

// ReplicaRefCount is how many rows in one table name a show, and whether
// deleting the show removes or rewrites them (Dropped) or keeps them as
// tombstones.
type ReplicaRefCount struct {
	Table   string
	Dropped bool
	Rows    int64
}

// ReplicatedShowGroup is every approved show sharing one (rooms, headliner,
// venue-local month and day) key across two or more venue-local years.
type ReplicatedShowGroup struct {
	VenueName     string
	HeadlinerName string
	// MonthDay is the shared venue-local MM-DD.
	MonthDay string
	// CurrentYear is the venue-local year of the planning instant.
	CurrentYear int
	// Copies are ordered by venue-local date, then show id.
	Copies []ReplicatedShowCopy
	// Siblings are shows of any other status with the group's key (rooms,
	// headliner, venue-local month and day). Any sibling leaves the group
	// untouched: the real original may be one of them.
	Siblings []ReplicatedShowCopy

	// KeepID and DeleteIDs are set only when SkipReason is empty.
	KeepID    uint
	DeleteIDs []uint
	// SkipReason says why the group is left untouched; empty means it is
	// acted on by PurgeReplicatedShowGroup.
	SkipReason string

	key string
}

// Actionable reports whether PurgeReplicatedShowGroup may delete from this
// group.
func (g ReplicatedShowGroup) Actionable() bool {
	return g.SkipReason == "" && g.KeepID != 0 && len(g.DeleteIDs) > 0
}

// ReplicatedShowPlan is the whole dry-run answer: the groups, and the shows
// dated past the bound that no group explains.
type ReplicatedShowPlan struct {
	BeyondMonths int
	BeyondCutoff time.Time
	Groups       []ReplicatedShowGroup
	// Beyond lists approved shows dated after BeyondCutoff that are in no
	// group. They are never deleted by this file.
	Beyond []ReplicatedShowCopy
}

// Digest names the plan's deletions: every actionable group's survivor and
// deletion set, and for each copy to delete the count of every row its delete
// removes (the Dropped refs), tables in sorted order; order-independent across
// groups. Two plans with the same digest delete the same shows, keep the same
// survivors and take the same counts of rows with them.
func (p *ReplicatedShowPlan) Digest() string {
	lines := make([]string, 0, len(p.Groups))
	for _, g := range p.Groups {
		if !g.Actionable() {
			continue
		}
		ids := make([]string, len(g.DeleteIDs))
		for i, id := range g.DeleteIDs {
			ids[i] = strconv.FormatUint(uint64(id), 10) + "[" + droppedRefsKey(g.copy(id)) + "]"
		}
		lines = append(lines, fmt.Sprintf("keep %d delete %s", g.KeepID, strings.Join(ids, ",")))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}

// copy returns the group's copy with the given id, or the zero copy.
func (g ReplicatedShowGroup) copy(id uint) ReplicatedShowCopy {
	for _, c := range g.Copies {
		if c.ShowID == id {
			return c
		}
	}
	return ReplicatedShowCopy{}
}

// droppedRefsKey is "table=n" for every ref the delete removes, sorted.
func droppedRefsKey(c ReplicatedShowCopy) string {
	var parts []string
	for _, r := range c.Refs {
		if r.Dropped {
			parts = append(parts, fmt.Sprintf("%s=%d", r.Table, r.Rows))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

// Skip reasons. Each names the condition that keeps a group untouched.
const (
	replicaSkipSibling          = "a show with the same rooms, headliner and month-day is not approved (listed as SIBLING), so the original may be one of them; decide by hand"
	replicaSkipMixedOrigin      = "the copies do not share one source and submitter"
	replicaSkipNoTicketURL      = "a copy has no ticket URL"
	replicaSkipTicketURLsDiffer = "the copies do not share one ticket URL"
	replicaSkipSameYear         = "two copies fall in the same venue-local year"
	replicaSkipNoCurrentYear    = "the venue-local current year could not be read"
	replicaSkipPastYear         = "a copy is dated in a venue-local year before the current one, so the earliest copy may be the original; decide by hand"
	replicaSkipNoNearCopy       = "no copy is in the current or the next venue-local year"
	replicaSkipNoFarCopy        = "no copy is dated past the far-future bound"
)

// The two window reasons state replicaCreatedWindow itself, so the printed
// threshold cannot drift from the one applied.
var (
	replicaSkipCreatedApart     = fmt.Sprintf("the copies to delete were not created within %s of each other", replicaCreatedWindow)
	replicaSkipPairCreatedApart = fmt.Sprintf("only one copy to delete, and it was not created within %s of the copy kept", replicaCreatedWindow)
)

// ticketURLTrimSQL is the ASCII whitespace set stripped from a stored
// ticket_url before it is compared: space, tab, newline, carriage return, form
// feed and vertical tab. The vertical tab is written as \x0b because a Postgres
// E-string reads \v as a literal "v".
const ticketURLTrimSQL = `E' \t\n\r\f\x0b'`

// replicaRowsSQL reads every show, of any status, with its grouping key. Rooms are the
// FULL sorted set of show_venues ids, so two shows group only when they are at
// exactly the same rooms. The headliner is the RESOLVE rule shared by the slug
// writer (set_type 'headliner' first, then lowest position, then lowest artist
// id), so a group's headliner is the act its slugs name.
//
// The single `?` is the planning instant, bound into
// shared.VenueLocalDateAtParamSQL.
var replicaRowsSQL = `
SELECT shows.id AS show_id,
       COALESCE(shows.slug, '') AS slug,
       shows.event_date AS event_date,
       shows.created_at AS created_at,
       COALESCE(btrim(shows.ticket_url, ` + ticketURLTrimSQL + `), '') AS ticket_url,
       COALESCE(shows.source::text, '') AS source,
       COALESCE(shows.submitted_by, 0) AS submitted_by,
       shows.status::text AS status,
       (` + shared.VenueLocalDateSQL + `)::text AS local_date,
       (` + shared.VenueLocalDateAtParamSQL + `)::text AS local_today,
       COALESCE(rooms.venue_ids, '') AS venue_ids,
       COALESCE(rooms.venue_name, '') AS venue_name,
       COALESCE(head.artist_id, 0) AS headliner_id,
       COALESCE(head.name, '') AS headliner_name
FROM shows
` + shared.VenueTZJoin + `
LEFT JOIN LATERAL (
    SELECT string_agg(sv.venue_id::text, ',' ORDER BY sv.venue_id) AS venue_ids,
           (array_agg(v.name ORDER BY sv.venue_id))[1] AS venue_name
    FROM show_venues sv
    JOIN venues v ON v.id = sv.venue_id
    WHERE sv.show_id = shows.id
) rooms ON true
LEFT JOIN LATERAL (
    SELECT sa.artist_id, a.name
    FROM show_artists sa
    JOIN artists a ON a.id = sa.artist_id
    WHERE sa.show_id = shows.id
    ORDER BY CASE WHEN sa.set_type = 'headliner' THEN 0 ELSE 1 END, sa.position ASC, sa.artist_id ASC
    LIMIT 1
) head ON true
WHERE true`

type replicaRow struct {
	ShowID        uint
	Slug          string
	EventDate     time.Time
	CreatedAt     time.Time
	TicketURL     string
	Source        string
	SubmittedBy   uint
	Status        string
	LocalDate     string
	LocalToday    string
	VenueIDs      string
	VenueName     string
	HeadlinerID   uint
	HeadlinerName string
}

func (r replicaRow) toCopy() ReplicatedShowCopy {
	return ReplicatedShowCopy{
		ShowID:        r.ShowID,
		Slug:          r.Slug,
		LocalDate:     r.LocalDate,
		EventDate:     r.EventDate,
		CreatedAt:     r.CreatedAt,
		TicketURL:     r.TicketURL,
		Source:        r.Source,
		SubmittedBy:   r.SubmittedBy,
		Status:        r.Status,
		VenueName:     r.VenueName,
		HeadlinerName: r.HeadlinerName,
		venueIDs:      r.VenueIDs,
		headlinerID:   r.HeadlinerID,
		localToday:    r.LocalToday,
	}
}

// groupKey is empty for a show that cannot be grouped: no room, no billed act,
// or a date the query could not read.
func (c ReplicatedShowCopy) groupKey() string {
	if c.venueIDs == "" || c.headlinerID == 0 || len(c.LocalDate) != len("2006-01-02") {
		return ""
	}
	return c.venueIDs + "|" + strconv.FormatUint(uint64(c.headlinerID), 10) + "|" + c.LocalDate[5:]
}

func scanReplicaCopies(db *gorm.DB, now time.Time, showIDs []uint) ([]ReplicatedShowCopy, error) {
	query := replicaRowsSQL
	args := []any{now}
	if showIDs != nil {
		query += ` AND shows.id IN ?`
		args = append(args, showIDs)
	}
	var rows []replicaRow
	if err := db.Raw(query+` ORDER BY shows.id`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read shows for replica grouping: %w", err)
	}
	copies := make([]ReplicatedShowCopy, len(rows))
	for i, r := range rows {
		copies[i] = r.toCopy()
	}
	return copies, nil
}

// groupReplicaCopies buckets copies (of any status) by key and classifies every
// bucket whose APPROVED copies span two or more venue-local years; the bucket's
// other copies become the group's Siblings. A bucket whose approved copies sit
// inside one year (a single show, or same-date duplicates) is not a replicated
// group, and only its approved copies are returned in loners. farBound is the
// far-future bound at the planning instant.
//
// The ticket URL is a condition on a bucket, not part of its key: a bucket whose
// copies carry different links is listed whole, untouched, rather than split
// into sub-groups a human would never see side by side.
func groupReplicaCopies(copies []ReplicatedShowCopy, farBound time.Time) (groups []ReplicatedShowGroup, loners []ReplicatedShowCopy) {
	buckets := make(map[string][]ReplicatedShowCopy)
	for _, c := range copies {
		key := c.groupKey()
		if key == "" {
			if c.Status == string(catalogm.ShowStatusApproved) {
				loners = append(loners, c)
			}
			continue
		}
		buckets[key] = append(buckets[key], c)
	}
	for key, all := range buckets {
		var members, siblings []ReplicatedShowCopy
		for _, c := range all {
			if c.Status == string(catalogm.ShowStatusApproved) {
				members = append(members, c)
			} else {
				siblings = append(siblings, c)
			}
		}
		years := make(map[int]bool, len(members))
		for _, c := range members {
			years[localYear(c.LocalDate)] = true
		}
		if len(years) < 2 {
			loners = append(loners, members...)
			continue
		}
		group := classifyReplicaGroup(key, members, farBound)
		if len(siblings) > 0 {
			sort.Slice(siblings, func(i, j int) bool { return siblings[i].LocalDate < siblings[j].LocalDate })
			group.Siblings = siblings
			group.SkipReason = replicaSkipSibling
			group.KeepID = 0
			group.DeleteIDs = nil
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.VenueName != b.VenueName {
			return a.VenueName < b.VenueName
		}
		if a.HeadlinerName != b.HeadlinerName {
			return a.HeadlinerName < b.HeadlinerName
		}
		if a.MonthDay != b.MonthDay {
			return a.MonthDay < b.MonthDay
		}
		return a.key < b.key
	})
	return groups, loners
}

// classifyReplicaGroup decides what happens to one group. The survivor is the
// copy in the earliest venue-local year, which must be the current or the next
// one; a copy dated earlier in the CURRENT year is a valid survivor even though
// its date has passed. Every condition in the skip list must hold before
// anything is deleted, and the first that fails is the reason reported.
func classifyReplicaGroup(key string, members []ReplicatedShowCopy, farBound time.Time) ReplicatedShowGroup {
	copies := slices.Clone(members)
	sort.Slice(copies, func(i, j int) bool {
		if copies[i].LocalDate != copies[j].LocalDate {
			return copies[i].LocalDate < copies[j].LocalDate
		}
		return copies[i].ShowID < copies[j].ShowID
	})

	first := copies[0]
	group := ReplicatedShowGroup{
		VenueName:     first.VenueName,
		HeadlinerName: first.HeadlinerName,
		MonthDay:      first.LocalDate[5:],
		CurrentYear:   localYear(first.localToday),
		Copies:        copies,
		key:           key,
	}
	skip := func(reason string) ReplicatedShowGroup {
		group.SkipReason = reason
		return group
	}

	seenYears := make(map[int]bool, len(copies))
	far := false
	for _, c := range copies {
		switch {
		case c.Source != first.Source || c.SubmittedBy != first.SubmittedBy:
			return skip(replicaSkipMixedOrigin)
		case c.TicketURL == "":
			return skip(replicaSkipNoTicketURL)
		case c.TicketURL != first.TicketURL:
			return skip(replicaSkipTicketURLsDiffer)
		case seenYears[localYear(c.LocalDate)]:
			return skip(replicaSkipSameYear)
		}
		seenYears[localYear(c.LocalDate)] = true
		far = far || c.EventDate.After(farBound)
	}

	// Copies are sorted by date, so the first copy holds the earliest year.
	firstYear := localYear(first.LocalDate)
	switch {
	case group.CurrentYear == 0:
		return skip(replicaSkipNoCurrentYear)
	case firstYear < group.CurrentYear:
		return skip(replicaSkipPastYear)
	case firstYear > group.CurrentYear+1:
		return skip(replicaSkipNoNearCopy)
	case !far:
		return skip(replicaSkipNoFarCopy)
	}

	doomed := copies[1:]
	// One copy to delete has no spread of its own to measure, so it must have
	// been written with the copy kept.
	if len(doomed) == 1 {
		gap := doomed[0].CreatedAt.Sub(first.CreatedAt)
		if gap < 0 {
			gap = -gap
		}
		if gap > replicaCreatedWindow {
			return skip(replicaSkipPairCreatedApart)
		}
	}
	earliest, latest := doomed[0].CreatedAt, doomed[0].CreatedAt
	for _, c := range doomed {
		if c.CreatedAt.Before(earliest) {
			earliest = c.CreatedAt
		}
		if c.CreatedAt.After(latest) {
			latest = c.CreatedAt
		}
	}
	if latest.Sub(earliest) > replicaCreatedWindow {
		return skip(replicaSkipCreatedApart)
	}

	group.KeepID = first.ShowID
	for _, c := range doomed {
		group.DeleteIDs = append(group.DeleteIDs, c.ShowID)
	}
	return group
}

// localYear reads the year of a YYYY-MM-DD string, or 0 when it cannot.
func localYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return year
}

// PlanReplicatedShowPurge reads every approved show and returns the replicated
// groups plus the shows dated more than beyondMonths after now that belong to
// no group. It writes nothing.
func PlanReplicatedShowPurge(db *gorm.DB, now time.Time, beyondMonths int) (*ReplicatedShowPlan, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if beyondMonths < 1 {
		return nil, fmt.Errorf("beyond months must be at least 1, got %d", beyondMonths)
	}

	copies, err := scanReplicaCopies(db, now, nil)
	if err != nil {
		return nil, err
	}

	groups, loners := groupReplicaCopies(copies, showEventDateBound(now))
	plan := &ReplicatedShowPlan{
		BeyondMonths: beyondMonths,
		BeyondCutoff: monthsAfter(now, beyondMonths),
		Groups:       groups,
	}
	for _, c := range loners {
		if c.EventDate.After(plan.BeyondCutoff) {
			plan.Beyond = append(plan.Beyond, c)
		}
	}
	sort.Slice(plan.Beyond, func(i, j int) bool {
		if !plan.Beyond[i].EventDate.Equal(plan.Beyond[j].EventDate) {
			return plan.Beyond[i].EventDate.Before(plan.Beyond[j].EventDate)
		}
		return plan.Beyond[i].ShowID < plan.Beyond[j].ShowID
	})

	if err := attachReplicaRefCounts(db, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// showFKRefs are the foreign keys to shows.id that a show delete cascades (or,
// for duplicate_of_show_id, nulls), other than show_artists and show_venues
// (snapshotted directly) and show_dedup_keys (derived by trigger from those
// two). TestShowFKRefsCoverTheSchema fails when a migration adds a foreign key
// to shows that is in neither this list nor that exempt set. Each entry is
// (label, table, column, dropped).
var showFKRefs = []struct {
	label, table, column string
	dropped              bool
}{
	{"show_reports (cascade)", "show_reports", "show_id", true},
	{"enrichment_queue (cascade)", "enrichment_queue", "show_id", true},
	{"show_notify_queue (cascade)", "show_notify_queue", "show_id", true},
	{"venue_show_alert_batch (cascade)", "venue_show_alert_batch", "show_id", true},
	{"shows.duplicate_of_show_id (set to NULL)", "shows", "duplicate_of_show_id", true},
}

// attachReplicaRefCounts fills each printed copy's Refs. The polymorphic part
// reads the same inventory DeleteShow sweeps (entityRefsWalkedOnDelete and its
// recorded dispositions) plus the alert rows that inventory cannot see; the
// foreign-key part reads showFKRefs.
func attachReplicaRefCounts(db *gorm.DB, plan *ReplicatedShowPlan) error {
	var ids []uint
	for _, g := range plan.Groups {
		for _, c := range g.Copies {
			ids = append(ids, c.ShowID)
		}
	}
	for _, c := range plan.Beyond {
		ids = append(ids, c.ShowID)
	}
	if len(ids) == 0 {
		return nil
	}

	refs := make(map[uint][]ReplicaRefCount)
	collect := func(label, query string, dropped bool, args ...any) error {
		var rows []struct {
			EntityID uint
			N        int64
		}
		if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("failed to count %s on replica copies: %w", label, err)
		}
		for _, r := range rows {
			refs[r.EntityID] = append(refs[r.EntityID], ReplicaRefCount{Table: label, Dropped: dropped, Rows: r.N})
		}
		return nil
	}
	// #nosec G201 -- tables and columns below come from hardcoded inventories.
	polymorphic := "SELECT %[2]s AS entity_id, COUNT(*) AS n FROM %[1]s WHERE entity_type IN ? AND %[2]s IN ? GROUP BY %[2]s"
	for _, ref := range entityRefsWalkedOnDelete() {
		disposition, ok := entityRefDeleteDispositions[ref.table]
		if !ok {
			return fmt.Errorf("%s has no recorded delete disposition", ref.table)
		}
		if err := collect(ref.table, fmt.Sprintf(polymorphic, ref.table, ref.idCol), disposition == dropRefRows,
			[]string{string(entityTypeShow)}, ids); err != nil {
			return err
		}
	}
	if err := collect("notification_log (alert discriminators)", fmt.Sprintf(polymorphic, "notification_log", "entity_id"), true,
		alertTypesKeyedOnEntity[entityTypeShow], ids); err != nil {
		return err
	}
	for _, fk := range showFKRefs {
		q := fmt.Sprintf("SELECT %[2]s AS entity_id, COUNT(*) AS n FROM %[1]s WHERE %[2]s IN ? GROUP BY %[2]s", fk.table, fk.column)
		if err := collect(fk.label, q, fk.dropped, ids); err != nil {
			return err
		}
	}

	for gi := range plan.Groups {
		for ci := range plan.Groups[gi].Copies {
			c := &plan.Groups[gi].Copies[ci]
			c.Refs = refs[c.ShowID]
		}
	}
	for i := range plan.Beyond {
		plan.Beyond[i].Refs = refs[plan.Beyond[i].ShowID]
	}
	return nil
}

// snapshotReplicaShows returns, per show id, one JSON document holding the shows
// row and every row the delete removes or rewrites (its bill, its rooms, the
// foreign-key cascades in showFKRefs, the ids of shows whose
// duplicate_of_show_id it nulls, the polymorphic rows dispositioned
// dropRefRows and its alert rows), so a deletion can be restored by hand. The
// documents carry user ids (bookmarks, reports, notification rows).
//
// Read inside the purge's transaction, after the lock. The lock on shows holds
// back new rows in the foreign-key tables, but not in the polymorphic tables,
// which have no foreign key: a polymorphic row committed after this read and
// before the delete's sweep is deleted without a copy.
func snapshotReplicaShows(tx *gorm.DB, ids []uint) ([]json.RawMessage, error) {
	parts := []string{
		"'show', to_jsonb(s)",
		"'show_artists', (SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM show_artists x WHERE x.show_id = s.id)",
		"'show_venues', (SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM show_venues x WHERE x.show_id = s.id)",
	}
	for _, fk := range showFKRefs {
		if fk.table == "shows" {
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"'%[1]s', (SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM %[1]s x WHERE x.%[2]s = s.id)",
			fk.table, fk.column))
	}
	parts = append(parts,
		"'shows_pointing_here', (SELECT COALESCE(jsonb_agg(x.id), '[]'::jsonb) FROM shows x WHERE x.duplicate_of_show_id = s.id)")
	for _, ref := range entityRefsWalkedOnDelete() {
		// Rows kept as tombstones stay in the database, so copying them would
		// restore nothing and only move users' text off it.
		if entityRefDeleteDispositions[ref.table] != dropRefRows {
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"'%[1]s', (SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM %[1]s x WHERE x.entity_type = 'show' AND x.%[2]s = s.id)",
			ref.table, ref.idCol))
	}
	alertTypes := alertTypesKeyedOnEntity[entityTypeShow]
	parts = append(parts,
		"'notification_log_alerts', (SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM notification_log x WHERE x.entity_type IN ? AND x.entity_id = s.id)")

	var rows []struct {
		ID  uint
		Doc string
	}
	// #nosec G201 -- every interpolated table and column comes from a hardcoded inventory.
	q := "SELECT s.id AS id, jsonb_build_object(" + strings.Join(parts, ", ") + ")::text AS doc FROM shows s WHERE s.id IN ? ORDER BY s.id"
	if err := tx.Raw(q, alertTypes, ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to snapshot replica copies: %w", err)
	}
	if len(rows) != len(ids) {
		return nil, fmt.Errorf("%w: snapshot found %d of %d shows", ErrReplicaGroupChanged, len(rows), len(ids))
	}
	out := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		out[i] = json.RawMessage(r.Doc)
	}
	return out, nil
}

// ErrReplicaGroupChanged is returned when a group's shows no longer read the way
// the plan described them, so nothing in that group is deleted.
var ErrReplicaGroupChanged = errors.New("replica group changed since it was planned")

// PurgeReplicatedShowGroup deletes a planned group's DeleteIDs in one
// transaction, through ShowService.DeleteShow.
//
// Before deleting, it locks every show in the group and re-derives the group
// from the rows as they now stand, with the plan's own instant. Anything that
// moved the survivor or the deletion set (a show deleted, unapproved, re-dated,
// re-billed, moved room or given another ticket URL) aborts the whole group
// with ErrReplicaGroupChanged and deletes nothing.
//
// backup receives one snapshot per show about to be deleted (see
// snapshotReplicaShows) before any delete runs; an error from it aborts the
// group, so nothing is deleted that was not first handed to backup.
func PurgeReplicatedShowGroup(db *gorm.DB, now time.Time, group ReplicatedShowGroup, backup func([]json.RawMessage) error) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	if backup == nil {
		return fmt.Errorf("a backup writer is required")
	}
	if !group.Actionable() {
		return fmt.Errorf("group %s at %s on %s is not actionable: %s",
			group.HeadlinerName, group.VenueName, group.MonthDay, group.SkipReason)
	}

	ids := make([]uint, 0, len(group.Copies))
	for _, c := range group.Copies {
		ids = append(ids, c.ShowID)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var locked []uint
		if err := tx.Raw(`SELECT id FROM shows WHERE id IN ? ORDER BY id FOR UPDATE`, ids).
			Scan(&locked).Error; err != nil {
			return fmt.Errorf("failed to lock replica group: %w", err)
		}

		copies, err := scanReplicaCopies(tx, now, ids)
		if err != nil {
			return err
		}
		if len(copies) != len(ids) {
			return fmt.Errorf("%w: %d of %d shows still exist", ErrReplicaGroupChanged, len(copies), len(ids))
		}
		fresh, loners := groupReplicaCopies(copies, showEventDateBound(now))
		if len(loners) != 0 || len(fresh) != 1 {
			return fmt.Errorf("%w: the shows no longer form one group", ErrReplicaGroupChanged)
		}
		if fresh[0].KeepID != group.KeepID || !slices.Equal(fresh[0].DeleteIDs, group.DeleteIDs) {
			return fmt.Errorf("%w: survivor or deletion set differs (now keep %d, delete %v; skip %q)",
				ErrReplicaGroupChanged, fresh[0].KeepID, fresh[0].DeleteIDs, fresh[0].SkipReason)
		}

		snapshot, err := snapshotReplicaShows(tx, group.DeleteIDs)
		if err != nil {
			return err
		}
		if err := backup(snapshot); err != nil {
			return fmt.Errorf("backup failed, nothing deleted: %w", err)
		}

		svc := NewShowService(tx)
		for _, id := range group.DeleteIDs {
			if err := svc.DeleteShow(id); err != nil {
				return fmt.Errorf("failed to delete show %d: %w", id, err)
			}
		}
		return nil
	})
}
