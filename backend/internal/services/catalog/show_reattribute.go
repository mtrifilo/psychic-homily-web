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
	"psychic-homily-backend/internal/utils"
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
//   - A merge is explicit. The named show is the winner and keeps its slug; the
//     row's show is deleted through MergeDuplicateShow, so its URL stops
//     answering.
//   - Every row runs in its own transaction. A preview runs the same statements
//     and rolls back, so a dry run and a confirmed run cannot disagree about
//     what a row does.
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

// ReattributionOutcome is what one row did, or would do under a preview.
type ReattributionOutcome string

const (
	// ReattributionMoved: the show's room was re-pointed.
	ReattributionMoved ReattributionOutcome = "moved"
	// ReattributionMerged: the show was merged into the named show and deleted.
	ReattributionMerged ReattributionOutcome = "merged"
	// ReattributionUnchanged: the show is already at the room; nothing to do.
	ReattributionUnchanged ReattributionOutcome = "unchanged"
	// ReattributionRefused: the row failed a check and nothing was written.
	ReattributionRefused ReattributionOutcome = "refused"
	// ReattributionFailed: the row passed its checks and the database rejected
	// the write; its transaction rolled back.
	ReattributionFailed ReattributionOutcome = "failed"
)

// ReattributionRoom is a room as the result reports it.
type ReattributionRoom struct {
	ID    uint
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
}

// ShowReattributionResult reports one row.
type ShowReattributionResult struct {
	Outcome ReattributionOutcome
	// Reason says why a row was refused or failed, or why it was unchanged.
	Reason string

	ShowID   uint
	ShowSlug string
	// Headliner is the act the show's slug and page lead with.
	Headliner   string
	CurrentRoom *ReattributionRoom
	TargetRoom  *ReattributionRoom
	// VenueLocalDate is the show's date in the target room's zone.
	VenueLocalDate string

	MergeIntoID   uint
	MergeIntoSlug string
	// ActsAddedToWinner are acts on the merged show's bill that the winner does
	// not bill yet. MergeDuplicateShow moves them onto the winner with their own
	// position and role, so a reviewer sees here what the winner's bill becomes.
	ActsAddedToWinner []string
	// MergeSummary is filled for a merge, with the counts of the merge itself.
	MergeSummary *ShowDedupSummary

	// SameNightAtTarget lists the other shows at the target room on the same
	// venue-local date. A plain move that leaves one of these behind is how a
	// duplicate is born, so the CLI prints them as a warning for the reviewer
	// to decide on. Never acted upon here.
	SameNightAtTarget []ReattributionNeighbour

	// Committed is true only when the row's transaction committed a change.
	Committed bool
}

// errReattributionRollback ends a row's transaction without committing, for a
// preview and for a row that writes nothing.
var errReattributionRollback = errors.New("reattribution: roll back")

// reattributionRefusal carries a refusal out of the transaction.
type reattributionRefusal struct{ reason string }

func (r *reattributionRefusal) Error() string { return r.reason }

func refuse(format string, args ...interface{}) error {
	return &reattributionRefusal{reason: fmt.Sprintf(format, args...)}
}

// ReattributeShow applies one reviewed row in its own transaction, or, with
// apply false, runs the same statements and rolls them back.
func ReattributeShow(db *gorm.DB, row ShowReattribution, apply bool) ShowReattributionResult {
	row.ShowSlug = strings.TrimSpace(row.ShowSlug)
	row.RoomSlug = strings.TrimSpace(row.RoomSlug)
	row.MergeIntoSlug = strings.TrimSpace(row.MergeIntoSlug)

	res := ShowReattributionResult{ShowSlug: row.ShowSlug, MergeIntoSlug: row.MergeIntoSlug}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := reattributeInTx(tx, row, &res); err != nil {
			return err
		}
		if res.Outcome == ReattributionUnchanged || !apply {
			return errReattributionRollback
		}
		return nil
	})

	var refusal *reattributionRefusal
	switch {
	case err == nil:
		res.Committed = true
	case errors.Is(err, errReattributionRollback):
	case errors.As(err, &refusal):
		res.Outcome = ReattributionRefused
		res.Reason = refusal.reason
	default:
		res.Outcome = ReattributionFailed
		res.Reason = err.Error()
	}
	if res.Outcome != ReattributionMerged {
		res.MergeSummary = nil
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

	// Lock the shows in ascending id order, one statement each (IN ignores
	// order), so two runs touching the same pair cannot deadlock.
	for _, id := range ascendingNonZero(showID, winnerID) {
		var locked catalogm.Show
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&locked, id).Error; err != nil {
			return fmt.Errorf("lock show %d: %w", id, err)
		}
	}

	var show catalogm.Show
	if err := tx.First(&show, showID).Error; err != nil {
		return fmt.Errorf("load show %d: %w", showID, err)
	}
	res.ShowID = show.ID
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

	zone := utils.EventLocation(target.Timezone, target.State)
	localDate := show.EventDate.In(zone).Format("2006-01-02")
	res.VenueLocalDate = localDate

	if winnerID != 0 {
		return mergeIntoCopy(tx, &show, winnerID, &target, zone, res)
	}

	if current.ID == target.ID {
		res.Outcome = ReattributionUnchanged
		res.Reason = "already at the target room"
		return nil
	}

	if res.SameNightAtTarget, err = showsAtRoomOnLocalDate(tx, target.ID, show.ID, localDate, zone); err != nil {
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
	if err := tx.Model(&catalogm.Show{}).Where("id = ?", show.ID).
		Update("updated_at", time.Now()).Error; err != nil {
		return fmt.Errorf("touch show: %w", err)
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
func mergeIntoCopy(tx *gorm.DB, loser *catalogm.Show, winnerID uint, target *catalogm.Venue,
	zone *time.Location, res *ShowReattributionResult) error {
	var winner catalogm.Show
	if err := tx.First(&winner, winnerID).Error; err != nil {
		return fmt.Errorf("load merge target %d: %w", winnerID, err)
	}
	res.MergeIntoID = winner.ID

	var atTarget int64
	if err := tx.Model(&catalogm.ShowVenue{}).
		Where("show_id = ? AND venue_id = ?", winner.ID, target.ID).
		Count(&atTarget).Error; err != nil {
		return fmt.Errorf("check merge target room: %w", err)
	}
	if atTarget == 0 {
		return refuse("merge target %s is not at room %s", res.MergeIntoSlug, venueSlug(target))
	}

	winnerDate := winner.EventDate.In(zone).Format("2006-01-02")
	if winnerDate != res.VenueLocalDate {
		return refuse("merge target is on %s and the show is on %s (venue-local)", winnerDate, res.VenueLocalDate)
	}

	added, err := actsMissingFromWinner(tx, loser.ID, winner.ID)
	if err != nil {
		return err
	}
	res.ActsAddedToWinner = added

	if err := tx.Exec(`DELETE FROM show_venues WHERE show_id = ?`, loser.ID).Error; err != nil {
		return fmt.Errorf("clear merged show's room: %w", err)
	}
	summary := &ShowDedupSummary{}
	if err := MergeDuplicateShow(tx, winner.ID, loser.ID, summary); err != nil {
		return fmt.Errorf("merge show %d into %d: %w", loser.ID, winner.ID, err)
	}
	res.MergeSummary = summary
	res.Outcome = ReattributionMerged
	return nil
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

// showHeadlinerName ranks the bill the way RecanonicaliseShowSlug does.
func showHeadlinerName(tx *gorm.DB, showID uint) (string, error) {
	var names []string
	if err := tx.Table("artists").
		Joins("JOIN show_artists ON show_artists.artist_id = artists.id").
		Where("show_artists.show_id = ?", showID).
		Order("CASE WHEN show_artists.set_type='headliner' THEN 0 ELSE 1 END, show_artists.position ASC, artists.id ASC").
		Limit(1).
		Pluck("artists.name", &names).Error; err != nil {
		return "", fmt.Errorf("load headliner of show %d: %w", showID, err)
	}
	if len(names) == 0 {
		return "", nil
	}
	return names[0], nil
}

func actsMissingFromWinner(tx *gorm.DB, loserID, winnerID uint) ([]string, error) {
	var names []string
	if err := tx.Raw(`
		SELECT a.name
		FROM show_artists l
		JOIN artists a ON a.id = l.artist_id
		WHERE l.show_id = ?
		  AND NOT EXISTS (SELECT 1 FROM show_artists w WHERE w.show_id = ? AND w.artist_id = l.artist_id)
		ORDER BY l.position, a.id
	`, loserID, winnerID).Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("compare bills of %d and %d: %w", loserID, winnerID, err)
	}
	return names, nil
}

// showsAtRoomOnLocalDate lists the other shows at a room whose event_date falls
// on localDate in zone. The SQL window is a coarse bound two days wide on each
// side; the exact venue-local comparison is made here.
func showsAtRoomOnLocalDate(tx *gorm.DB, roomID, exceptShowID uint, localDate string,
	zone *time.Location) ([]ReattributionNeighbour, error) {
	day, err := time.ParseInLocation("2006-01-02", localDate, zone)
	if err != nil {
		return nil, fmt.Errorf("parse local date %s: %w", localDate, err)
	}
	var rows []struct {
		ID        uint
		Slug      *string
		EventDate time.Time
	}
	if err := tx.Raw(`
		SELECT s.id, s.slug, s.event_date
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		WHERE sv.venue_id = ? AND s.id <> ?
		  AND s.event_date >= ? AND s.event_date < ?
		ORDER BY s.event_date, s.id
	`, roomID, exceptShowID, day.AddDate(0, 0, -2), day.AddDate(0, 0, 3)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("shows at room %d near %s: %w", roomID, localDate, err)
	}
	var out []ReattributionNeighbour
	for _, r := range rows {
		if r.EventDate.In(zone).Format("2006-01-02") != localDate {
			continue
		}
		headliner, err := showHeadlinerName(tx, r.ID)
		if err != nil {
			return nil, err
		}
		n := ReattributionNeighbour{ID: r.ID, Headliner: headliner}
		if r.Slug != nil {
			n.Slug = *r.Slug
		}
		out = append(out, n)
	}
	return out, nil
}

// sameCity compares the way the catalogue audit's same-night statement does:
// trimmed and case-folded city and state, plus country when both rooms state
// one.
func sameCity(a, b *catalogm.Venue) bool {
	fold := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	if fold(a.City) != fold(b.City) || fold(a.State) != fold(b.State) {
		return false
	}
	if a.Country != nil && b.Country != nil && fold(*a.Country) != "" && fold(*b.Country) != "" {
		return fold(*a.Country) == fold(*b.Country)
	}
	return true
}

func reportRoom(v *catalogm.Venue) *ReattributionRoom {
	return &ReattributionRoom{ID: v.ID, Slug: venueSlug(v), Name: v.Name, City: v.City, State: v.State}
}

func venueSlug(v *catalogm.Venue) string {
	if v.Slug == nil {
		return ""
	}
	return *v.Slug
}
