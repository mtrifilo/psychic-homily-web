package catalog

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/shared"
)

// A year-replicated show is one event written once per year: the same rooms,
// the same headliner, the same ticket URL and the same venue-local month and
// day, in several venue-local years. This file finds those groups and removes
// every copy but one through DeleteShow, so each removal runs the same
// reference sweep an admin delete does.
//
// The planner is deliberately narrow. A group is only acted on when nothing
// about it is ambiguous; everything else is reported with the reason it was
// left alone, for a human to decide.

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

	// The rows the delete path drops (bookmarks, crate items) or keeps as
	// tombstones (comments), counted so a reviewer sees what a copy carries.
	Bookmarks       int64
	CollectionItems int64
	Comments        int64

	venueIDs    string
	headlinerID uint
	localToday  string
}

// ReplicatedShowGroup is every approved show sharing one (rooms, headliner,
// venue-local month and day) key, two or more of them.
type ReplicatedShowGroup struct {
	VenueName     string
	HeadlinerName string
	// MonthDay is the shared venue-local MM-DD.
	MonthDay string
	// CurrentYear is the venue-local year of the planning instant.
	CurrentYear int
	// Copies are ordered by venue-local date, then show id.
	Copies []ReplicatedShowCopy

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
	Now          time.Time
	BeyondMonths int
	BeyondCutoff time.Time
	Groups       []ReplicatedShowGroup
	// Beyond lists approved shows dated after BeyondCutoff that are in no
	// group. They are never deleted by this file.
	Beyond []ReplicatedShowCopy
}

// Skip reasons. Each names the condition that keeps a group untouched.
const (
	replicaSkipNoTicketURL      = "a copy has no ticket URL"
	replicaSkipTicketURLsDiffer = "the copies do not share one ticket URL"
	replicaSkipSameYear         = "two copies fall in the same venue-local year"
	replicaSkipPastYear         = "a copy is dated in a venue-local year before the current one"
	replicaSkipNoNearCopy       = "no copy is in the current or the next venue-local year"
)

// replicaRowsSQL reads every approved show with its grouping key. Rooms are the
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
       COALESCE(btrim(shows.ticket_url), '') AS ticket_url,
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
WHERE shows.status = 'approved'`

type replicaRow struct {
	ShowID        uint
	Slug          string
	EventDate     time.Time
	CreatedAt     time.Time
	TicketURL     string
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

func scanReplicaRows(db *gorm.DB, now time.Time, showIDs []uint) ([]replicaRow, error) {
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
	return rows, nil
}

// groupReplicaCopies buckets copies by key and classifies every bucket that
// spans two or more venue-local years. A bucket inside one year (a single
// show, or same-date duplicates) is not a replicated group and its copies are
// returned in loners.
func groupReplicaCopies(copies []ReplicatedShowCopy) (groups []ReplicatedShowGroup, loners []ReplicatedShowCopy) {
	buckets := make(map[string][]ReplicatedShowCopy)
	for _, c := range copies {
		key := c.groupKey()
		if key == "" {
			loners = append(loners, c)
			continue
		}
		buckets[key] = append(buckets[key], c)
	}
	for key, members := range buckets {
		years := make(map[int]bool, len(members))
		for _, c := range members {
			years[localYear(c.LocalDate)] = true
		}
		if len(years) < 2 {
			loners = append(loners, members...)
			continue
		}
		groups = append(groups, classifyReplicaGroup(key, members))
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
// copy in the earliest venue-local year that is not before the current year,
// and the group is acted on only when that year is the current or the next one,
// every copy carries the same non-empty ticket URL, no two copies share a year,
// and no copy sits in a year before the current one. A copy dated earlier in the
// CURRENT year is a valid survivor even though its date has passed.
func classifyReplicaGroup(key string, members []ReplicatedShowCopy) ReplicatedShowGroup {
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

	url := first.TicketURL
	seenYears := make(map[int]bool, len(copies))
	for _, c := range copies {
		if c.TicketURL == "" {
			group.SkipReason = replicaSkipNoTicketURL
			return group
		}
		if c.TicketURL != url {
			group.SkipReason = replicaSkipTicketURLsDiffer
			return group
		}
		year := localYear(c.LocalDate)
		if seenYears[year] {
			group.SkipReason = replicaSkipSameYear
			return group
		}
		seenYears[year] = true
	}

	if group.CurrentYear == 0 {
		group.SkipReason = replicaSkipNoNearCopy
		return group
	}
	for _, c := range copies {
		if localYear(c.LocalDate) < group.CurrentYear {
			group.SkipReason = replicaSkipPastYear
			return group
		}
	}

	// Copies are sorted by date and every year is distinct and not past, so the
	// first copy is the earliest year that is not before the current one.
	if localYear(first.LocalDate) > group.CurrentYear+1 {
		group.SkipReason = replicaSkipNoNearCopy
		return group
	}
	group.KeepID = first.ShowID
	for _, c := range copies[1:] {
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

	rows, err := scanReplicaRows(db, now, nil)
	if err != nil {
		return nil, err
	}
	copies := make([]ReplicatedShowCopy, len(rows))
	for i, r := range rows {
		copies[i] = r.toCopy()
	}

	groups, loners := groupReplicaCopies(copies)
	plan := &ReplicatedShowPlan{
		Now:          now,
		BeyondMonths: beyondMonths,
		BeyondCutoff: now.UTC().AddDate(0, beyondMonths, 0),
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

// attachReplicaRefCounts fills the per-copy reference counts for every show the
// plan prints.
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

	counts := make(map[string]map[uint]int64, 3)
	for _, table := range []string{"user_bookmarks", "collection_items", "comments"} {
		var rows []struct {
			EntityID uint
			N        int64
		}
		// #nosec G201 -- table comes from the literal list above.
		q := fmt.Sprintf(
			"SELECT entity_id, COUNT(*) AS n FROM %s WHERE entity_type = 'show' AND entity_id IN ? GROUP BY entity_id",
			table)
		if err := db.Raw(q, ids).Scan(&rows).Error; err != nil {
			return fmt.Errorf("failed to count %s on replica copies: %w", table, err)
		}
		byID := make(map[uint]int64, len(rows))
		for _, r := range rows {
			byID[r.EntityID] = r.N
		}
		counts[table] = byID
	}

	fill := func(c *ReplicatedShowCopy) {
		c.Bookmarks = counts["user_bookmarks"][c.ShowID]
		c.CollectionItems = counts["collection_items"][c.ShowID]
		c.Comments = counts["comments"][c.ShowID]
	}
	for gi := range plan.Groups {
		for ci := range plan.Groups[gi].Copies {
			fill(&plan.Groups[gi].Copies[ci])
		}
	}
	for i := range plan.Beyond {
		fill(&plan.Beyond[i])
	}
	return nil
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
func PurgeReplicatedShowGroup(db *gorm.DB, now time.Time, group ReplicatedShowGroup) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
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

		rows, err := scanReplicaRows(tx, now, ids)
		if err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return fmt.Errorf("%w: %d of %d shows are still approved", ErrReplicaGroupChanged, len(rows), len(ids))
		}
		copies := make([]ReplicatedShowCopy, len(rows))
		for i, r := range rows {
			copies[i] = r.toCopy()
		}
		fresh, loners := groupReplicaCopies(copies)
		if len(loners) != 0 || len(fresh) != 1 {
			return fmt.Errorf("%w: the shows no longer form one group", ErrReplicaGroupChanged)
		}
		if fresh[0].KeepID != group.KeepID || !slices.Equal(fresh[0].DeleteIDs, group.DeleteIDs) {
			return fmt.Errorf("%w: survivor or deletion set differs (now keep %d, delete %v; skip %q)",
				ErrReplicaGroupChanged, fresh[0].KeepID, fresh[0].DeleteIDs, fresh[0].SkipReason)
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
