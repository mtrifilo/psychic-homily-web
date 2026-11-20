package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

// This file moves a show that was recorded at the wrong room onto the right
// one, and folds it into a copy already at the right room when a reviewer names
// that copy. It is driven by a human-reviewed list (cmd/reattribute-shows);
// nothing here decides on its own which room a show belongs in or which two
// shows are the same event.
//
// Invariants a reader can check against the code below:
//
//   - A moved show keeps its slug. The slug then names the old room, exactly as
//     after the admin room edit (replaceShowVenues), and the show's URL keeps
//     answering.
//   - A merge is explicit. The named show is the winner and keeps its slug,
//     its headliner and its bill order; the row's show is deleted through
//     MergeDuplicateShow, so its URL stops answering. Both shows must be
//     approved: a merge publishes the loser's comments, tags and bill on the
//     winner, and hides the loser's saves and comments behind a gated winner.
//   - Every row runs in its own transaction (a savepoint when the caller passes
//     one in). A caller previews by passing a transaction it rolls back, so a
//     preview runs exactly the statements a real run does.
//   - Nothing crosses a city. The room a show moves to must be in the city and
//     state of the room it leaves.

// ShowReattribution is one reviewed row: the show to move, the room it belongs
// in, and optionally the show already at that room it duplicates.
type ShowReattribution struct {
	ShowSlug string
	RoomSlug string
	// MergeIntoSlug names the show the row's show is merged into. Empty means a
	// plain move.
	MergeIntoSlug string
}

// ReattributionOutcome is what one row did.
type ReattributionOutcome string

const (
	// ReattributionMoved: the show's room was re-pointed.
	ReattributionMoved ReattributionOutcome = "moved"
	// ReattributionMerged: the show was merged into the named show and deleted.
	ReattributionMerged ReattributionOutcome = "merged"
	// ReattributionUnchanged: the show is already at the room; nothing written.
	ReattributionUnchanged ReattributionOutcome = "unchanged"
	// ReattributionRefused: the row failed a check and nothing was written.
	ReattributionRefused ReattributionOutcome = "refused"
	// ReattributionFailed: the row passed its checks and the database rejected
	// the write; its transaction rolled back.
	ReattributionFailed ReattributionOutcome = "failed"
)

// ReattributionRoom is a room as the result reports it.
type ReattributionRoom struct {
	Slug  string
	Name  string
	City  string
	State string
}

// ReattributionNeighbour is another show at the target room on the moved
// show's venue-local date.
type ReattributionNeighbour struct {
	ID        uint
	Slug      string
	Headliner string
	Status    catalogm.ShowStatus
}

// ReattributionAct is one act the merge adds to the winner's bill, with the
// role and position it lands at.
type ReattributionAct struct {
	Name     string
	SetType  string
	Position int
}

// ShowReattributionResult reports one row.
type ShowReattributionResult struct {
	Outcome ReattributionOutcome
	// Reason says why a row was refused or failed, or why it was unchanged.
	Reason string

	ShowID     uint
	ShowSlug   string
	ShowStatus catalogm.ShowStatus
	// Headliner is the act the show's slug and page lead with.
	Headliner   string
	CurrentRoom *ReattributionRoom
	TargetRoom  *ReattributionRoom
	// VenueLocalDate is the show's date in the target room's zone.
	VenueLocalDate string

	MergeIntoID        uint
	MergeIntoSlug      string
	MergeIntoStatus    catalogm.ShowStatus
	MergeIntoHeadliner string
	// ActsAddedToWinner are the acts on the merged show's bill that the winner
	// does not bill yet, as they land: after the winner's last act, with a
	// headliner role turned into performer.
	ActsAddedToWinner []ReattributionAct
	// MergeSummary is set only when Outcome is merged.
	MergeSummary *ShowDedupSummary

	// SameNightAtTarget lists the other shows at the target room on the same
	// venue-local date. A plain move that leaves one of these behind is how a
	// duplicate is born, so the CLI prints them as a warning for the reviewer
	// to decide on. Never acted upon here.
	SameNightAtTarget []ReattributionNeighbour
}

// reattributionRefusal carries a refusal out of the transaction.
type reattributionRefusal struct{ reason string }

func (r *reattributionRefusal) Error() string { return r.reason }

func refuse(format string, args ...interface{}) error {
	return &reattributionRefusal{reason: fmt.Sprintf(format, args...)}
}

// ReattributeShow applies one reviewed row in its own transaction, nested as a
// savepoint when db is already a transaction. A refused or failed row writes
// nothing.
func ReattributeShow(db *gorm.DB, row ShowReattribution) ShowReattributionResult {
	row.ShowSlug = strings.TrimSpace(row.ShowSlug)
	row.RoomSlug = strings.TrimSpace(row.RoomSlug)
	row.MergeIntoSlug = strings.TrimSpace(row.MergeIntoSlug)

	res := ShowReattributionResult{ShowSlug: row.ShowSlug, MergeIntoSlug: row.MergeIntoSlug}
	err := db.Transaction(func(tx *gorm.DB) error {
		return reattributeInTx(tx, row, &res)
	})

	var refusal *reattributionRefusal
	switch {
	case err == nil:
	case errors.As(err, &refusal):
		res.Outcome = ReattributionRefused
		res.Reason = refusal.reason
	default:
		res.Outcome = ReattributionFailed
		res.Reason = err.Error()
	}
	return res
}

func reattributeInTx(tx *gorm.DB, row ShowReattribution, res *ShowReattributionResult) error {
	if row.ShowSlug == "" {
		return refuse("no show slug")
	}
	if row.RoomSlug == "" {
		return refuse("no target room slug")
	}
	if row.MergeIntoSlug == row.ShowSlug {
		return refuse("merge target is the show itself")
	}

	showID, err := showIDBySlug(tx, row.ShowSlug)
	if err != nil {
		return err
	}
	var winnerID uint
	if row.MergeIntoSlug != "" {
		winnerID, err = showIDBySlug(tx, row.MergeIntoSlug)
		if err != nil {
			return err
		}
	}
	if showID == 0 {
		if winnerID != 0 {
			return refuse("show not found; the merge target %s exists, so this row may have been merged by an earlier run",
				row.MergeIntoSlug)
		}
		return refuse("show not found")
	}
	if row.MergeIntoSlug != "" && winnerID == 0 {
		return refuse("merge target %s not found", row.MergeIntoSlug)
	}

	// Lock and load the shows in ascending id order, one statement each (IN
	// ignores order), so two runs touching the same pair cannot deadlock.
	locked := map[uint]*catalogm.Show{}
	for _, id := range ascendingNonZero(showID, winnerID) {
		var s catalogm.Show
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, id).Error; err != nil {
			return fmt.Errorf("lock show %d: %w", id, err)
		}
		locked[id] = &s
	}
	show := locked[showID]
	res.ShowID = show.ID
	res.ShowStatus = show.Status
	if res.Headliner, err = showHeadlinerName(tx, show.ID); err != nil {
		return err
	}

	var target catalogm.Venue
	if err := tx.Where("slug = ?", row.RoomSlug).Limit(1).Find(&target).Error; err != nil {
		return fmt.Errorf("load room %s: %w", row.RoomSlug, err)
	}
	if target.ID == 0 {
		return refuse("room %s not found", row.RoomSlug)
	}
	res.TargetRoom = reportRoom(&target)

	rooms, err := showRooms(tx, show.ID)
	if err != nil {
		return err
	}
	if len(rooms) != 1 {
		return refuse("show lists %d rooms; only a show with exactly one room is moved", len(rooms))
	}
	current := rooms[0]
	res.CurrentRoom = reportRoom(&current)

	if !sameCity(&current, &target) {
		return refuse("target room is in %s, %s and the show's room is in %s, %s",
			target.City, target.State, current.City, current.State)
	}

	localDate := venueRowLocalDate(show.EventDate, target.Timezone, target.State)
	res.VenueLocalDate = localDate

	if winnerID != 0 {
		return mergeIntoCopy(tx, show, locked[winnerID], &target, localDate, res)
	}

	if current.ID == target.ID {
		res.Outcome = ReattributionUnchanged
		res.Reason = "already at the target room"
		return nil
	}

	if res.SameNightAtTarget, err = showsAtRoomOnLocalDate(tx, &target, show.ID, localDate); err != nil {
		return err
	}

	if err := tx.Exec(`UPDATE show_venues SET venue_id = ? WHERE show_id = ? AND venue_id = ?`,
		target.ID, show.ID, current.ID).Error; err != nil {
		return fmt.Errorf("re-point room: %w", err)
	}
	// show_artists carries the primary room denormalized for the dedup index;
	// the re-point above changes the primary room.
	if err := syncShowArtistDedupColumns(tx, show.ID); err != nil {
		return fmt.Errorf("show_artists dedup-column resync: %w", err)
	}
	if err := stampShowUpdatedAt(tx, show); err != nil {
		return err
	}
	res.Outcome = ReattributionMoved
	return nil
}

// mergeIntoCopy folds the row's show into the named winner, which must already
// be at the target room on the same venue-local date.
//
// The loser's room rows are removed BEFORE the merge. MergeDuplicateShow
// re-points the loser's show_venues onto the winner, which here would hand the
// winner the wrong room as a second room. With them gone the loser holds no
// room and so no show_dedup_keys, and its bill can move onto the winner
// without colliding.
func mergeIntoCopy(tx *gorm.DB, loser, winner *catalogm.Show, target *catalogm.Venue,
	localDate string, res *ShowReattributionResult) error {
	res.MergeIntoID = winner.ID
	res.MergeIntoStatus = winner.Status
	var err error
	if res.MergeIntoHeadliner, err = showHeadlinerName(tx, winner.ID); err != nil {
		return err
	}
	if loser.Status != catalogm.ShowStatusApproved || winner.Status != catalogm.ShowStatusApproved {
		return refuse("both shows must be approved to merge; the show is %s and the merge target is %s",
			loser.Status, winner.Status)
	}

	var atTarget int64
	if err := tx.Model(&catalogm.ShowVenue{}).
		Where("show_id = ? AND venue_id = ?", winner.ID, target.ID).
		Count(&atTarget).Error; err != nil {
		return fmt.Errorf("check merge target room: %w", err)
	}
	if atTarget == 0 {
		return refuse("merge target %s is not at room %s",
			shared.DerefOrEmpty(winner.Slug), shared.DerefOrEmpty(target.Slug))
	}

	winnerDate := venueRowLocalDate(winner.EventDate, target.Timezone, target.State)
	if winnerDate != localDate {
		return refuse("merge target is on %s and the show is on %s (venue-local)", winnerDate, localDate)
	}

	if err := tx.Exec(`DELETE FROM show_venues WHERE show_id = ?`, loser.ID).Error; err != nil {
		return fmt.Errorf("clear merged show's room: %w", err)
	}
	if res.ActsAddedToWinner, err = appendLoserActsToWinnerBill(tx, loser.ID, winner.ID); err != nil {
		return err
	}
	summary := &ShowDedupSummary{}
	if err := MergeDuplicateShow(tx, winner.ID, loser.ID, summary); err != nil {
		return fmt.Errorf("merge show %d into %d: %w", loser.ID, winner.ID, err)
	}
	if err := stampShowUpdatedAt(tx, winner); err != nil {
		return err
	}
	res.MergeSummary = summary
	res.Outcome = ReattributionMerged
	return nil
}

// appendLoserActsToWinnerBill rewrites, on the loser, the bill rows of acts the
// winner does not bill, so that MergeDuplicateShow moves them in BEHIND the
// winner's bill: positions after the winner's last one, in the loser's order,
// and a headliner role turned into performer. MergeDuplicateShow moves bill
// rows as they stand, so without this the loser's headliner would arrive as a
// second headliner at position 0 and could outrank the winner's own. Acts the
// winner already bills are left alone; the merge keeps the winner's row for
// them. Runs after the loser's rooms are cleared, so the loser holds no dedup
// keys these updates could collide with.
func appendLoserActsToWinnerBill(tx *gorm.DB, loserID, winnerID uint) ([]ReattributionAct, error) {
	var acts []ReattributionAct
	if err := tx.Raw(`
		WITH next AS (
			SELECT COALESCE(MAX(position), -1) + 1 AS base FROM show_artists WHERE show_id = ?
		), moving AS (
			SELECT l.artist_id,
			       ROW_NUMBER() OVER (ORDER BY l.position, l.artist_id) - 1 AS ord
			FROM show_artists l
			WHERE l.show_id = ?
			  AND NOT EXISTS (SELECT 1 FROM show_artists w WHERE w.show_id = ? AND w.artist_id = l.artist_id)
		)
		UPDATE show_artists sa
		SET position = next.base + moving.ord,
		    set_type = CASE WHEN sa.set_type = 'headliner' THEN 'performer' ELSE sa.set_type END
		FROM moving, next, artists a
		WHERE sa.show_id = ? AND sa.artist_id = moving.artist_id AND a.id = sa.artist_id
		RETURNING a.name AS name, sa.set_type AS set_type, sa.position AS position
	`, winnerID, loserID, winnerID, loserID).Scan(&acts).Error; err != nil {
		return nil, fmt.Errorf("place merged show's acts on show %d: %w", winnerID, err)
	}
	sort.Slice(acts, func(i, j int) bool { return acts[i].Position < acts[j].Position })
	return acts, nil
}

// showIDBySlug returns 0 with no error when no show has the slug.
func showIDBySlug(tx *gorm.DB, slug string) (uint, error) {
	var ids []uint
	if err := tx.Model(&catalogm.Show{}).Where("slug = ?", slug).Limit(1).Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("find show %s: %w", slug, err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func ascendingNonZero(ids ...uint) []uint {
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func showRooms(tx *gorm.DB, showID uint) ([]catalogm.Venue, error) {
	var rooms []catalogm.Venue
	if err := tx.Table("venues").
		Joins("JOIN show_venues ON show_venues.venue_id = venues.id").
		Where("show_venues.show_id = ?", showID).
		Order("venues.id ASC").
		Find(&rooms).Error; err != nil {
		return nil, fmt.Errorf("load rooms of show %d: %w", showID, err)
	}
	return rooms, nil
}

// showHeadlinerName ranks the bill with showBillHeadlinerRank, as the slug
// writer does.
func showHeadlinerName(tx *gorm.DB, showID uint) (string, error) {
	var names []string
	if err := tx.Table("artists").
		Joins("JOIN show_artists ON show_artists.artist_id = artists.id").
		Where("show_artists.show_id = ?", showID).
		Order(showBillHeadlinerRank).
		Limit(1).
		Pluck("artists.name", &names).Error; err != nil {
		return "", fmt.Errorf("load headliner of show %d: %w", showID, err)
	}
	if len(names) == 0 {
		return "", nil
	}
	return names[0], nil
}

// showsAtRoomOnLocalDate lists the other shows at a room whose event_date falls
// on localDate in the room's zone. The SQL window is a coarse bound a day wider
// than any UTC offset on each side; the exact venue-local comparison is made
// here, in the same zone the moved show's date was read in.
func showsAtRoomOnLocalDate(tx *gorm.DB, room *catalogm.Venue, exceptShowID uint,
	localDate string) ([]ReattributionNeighbour, error) {
	day, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return nil, fmt.Errorf("parse local date %s: %w", localDate, err)
	}
	var rows []struct {
		ID        uint
		Slug      *string
		Status    catalogm.ShowStatus
		EventDate time.Time
		Headliner *string
	}
	if err := tx.Raw(`
		SELECT s.id, s.slug, s.status, s.event_date, h.name AS headliner
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN LATERAL (
			SELECT artists.name
			FROM show_artists JOIN artists ON artists.id = show_artists.artist_id
			WHERE show_artists.show_id = s.id
			ORDER BY `+showBillHeadlinerRank+`
			LIMIT 1
		) h ON TRUE
		WHERE sv.venue_id = ? AND s.id <> ?
		  AND s.event_date >= ? AND s.event_date < ?
		ORDER BY s.event_date, s.id
	`, room.ID, exceptShowID, day.AddDate(0, 0, -2), day.AddDate(0, 0, 3)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("shows at room %d near %s: %w", room.ID, localDate, err)
	}
	var out []ReattributionNeighbour
	for _, r := range rows {
		if venueRowLocalDate(r.EventDate, room.Timezone, room.State) != localDate {
			continue
		}
		out = append(out, ReattributionNeighbour{
			ID: r.ID, Slug: shared.DerefOrEmpty(r.Slug), Headliner: shared.DerefOrEmpty(r.Headliner), Status: r.Status})
	}
	return out, nil
}

// sameCity compares trimmed and case-folded city and state, plus country when
// both rooms state one.
func sameCity(a, b *catalogm.Venue) bool {
	fold := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	if fold(a.City) != fold(b.City) || fold(a.State) != fold(b.State) {
		return false
	}
	ac, bc := fold(shared.DerefOrEmpty(a.Country)), fold(shared.DerefOrEmpty(b.Country))
	return ac == "" || bc == "" || ac == bc
}

func reportRoom(v *catalogm.Venue) *ReattributionRoom {
	return &ReattributionRoom{Slug: shared.DerefOrEmpty(v.Slug), Name: v.Name, City: v.City, State: v.State}
}
