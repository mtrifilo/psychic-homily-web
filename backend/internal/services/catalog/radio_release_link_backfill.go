package catalog

import (
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// RadioReleaseLinkOutcome classifies one existing radio_plays.release_id under
// pickReleaseLink.
type RadioReleaseLinkOutcome string

const (
	// ReleaseLinkKeptCredited: the play has a linked artist and the release is
	// credited to it.
	ReleaseLinkKeptCredited RadioReleaseLinkOutcome = "kept_credited"
	// ReleaseLinkKeptUniqueTitle: the play has no linked artist and the release
	// is the one pickReleaseLink chooses for it.
	ReleaseLinkKeptUniqueTitle RadioReleaseLinkOutcome = "kept_unique_title"
	// ReleaseLinkKeptAdminSet: an admin set this release on this play through
	// the link-play endpoint, so the backfill leaves it alone.
	ReleaseLinkKeptAdminSet RadioReleaseLinkOutcome = "kept_admin_set"
	// ReleaseLinkRemovedNotCredited: the play has a linked artist, the release
	// is not credited to it, and no release with the title is.
	ReleaseLinkRemovedNotCredited RadioReleaseLinkOutcome = "removed_not_credited"
	// ReleaseLinkRemovedUnlinkedArtist: the play has no linked artist and
	// pickReleaseLink chooses no release for it.
	ReleaseLinkRemovedUnlinkedArtist RadioReleaseLinkOutcome = "removed_unlinked_artist"
	// ReleaseLinkRelinkedCredited: the play has a linked artist, the release is
	// not credited to it, and another release with the title is.
	ReleaseLinkRelinkedCredited RadioReleaseLinkOutcome = "relinked_credited"
	// ReleaseLinkRelinkedUniqueTitle: the play has no linked artist and
	// pickReleaseLink chooses a different release for it.
	ReleaseLinkRelinkedUniqueTitle RadioReleaseLinkOutcome = "relinked_unique_title"
)

// RadioReleaseLinkOutcomes lists every outcome in report order.
var RadioReleaseLinkOutcomes = []RadioReleaseLinkOutcome{
	ReleaseLinkKeptCredited,
	ReleaseLinkKeptUniqueTitle,
	ReleaseLinkKeptAdminSet,
	ReleaseLinkRemovedNotCredited,
	ReleaseLinkRemovedUnlinkedArtist,
	ReleaseLinkRelinkedCredited,
	ReleaseLinkRelinkedUniqueTitle,
}

// IsRemoval reports whether the outcome clears the play's release link.
func (o RadioReleaseLinkOutcome) IsRemoval() bool {
	return o == ReleaseLinkRemovedNotCredited || o == ReleaseLinkRemovedUnlinkedArtist
}

// IsRelink reports whether the outcome points the play at another release.
func (o RadioReleaseLinkOutcome) IsRelink() bool {
	return o == ReleaseLinkRelinkedCredited || o == ReleaseLinkRelinkedUniqueTitle
}

// adminLinkPlayAuditAction is the audit_logs.action the admin link-play
// endpoint records, with the release it set in metadata.release_id.
const adminLinkPlayAuditAction = "link_radio_play"

// RadioReleaseLinkChange is one play whose release link the backfill removes
// or replaces.
type RadioReleaseLinkChange struct {
	PlayID                uint
	Outcome               RadioReleaseLinkOutcome
	ArtistID              *uint
	ArtistName            string
	AlbumTitle            string
	CurrentReleaseID      uint
	CurrentReleaseTitle   string
	CurrentReleaseArtists []string
	// NewReleaseID and NewReleaseTitle are set for relinks only.
	NewReleaseID    *uint
	NewReleaseTitle string
}

// RadioReleaseLinkReport is the result of one backfill run. Changes holds every
// removal and relink, ordered by play id.
type RadioReleaseLinkReport struct {
	Scanned int
	Counts  map[RadioReleaseLinkOutcome]int
	Changes []RadioReleaseLinkChange
	// Cleared and Relinked count rows written; both stay zero on a dry run.
	Cleared  int64
	Relinked int64
}

// RadioReleaseLinkBackfillOptions controls BackfillRadioReleaseLinks.
type RadioReleaseLinkBackfillOptions struct {
	// DryRun evaluates inside a READ ONLY transaction and writes nothing.
	DryRun bool
	// Limit caps the plays scanned, lowest id first; 0 scans all.
	Limit int
}

// BackfillRadioReleaseLinks re-evaluates every play's release_id under
// pickReleaseLink. Unless DryRun, it clears the links the rule refuses and
// points relinks at the rule's release, touching only release_id and only on
// rows whose release_id is still the one evaluated. Plays the admin link-play
// endpoint linked to their current release are kept as they are.
func BackfillRadioReleaseLinks(db *gorm.DB, opts RadioReleaseLinkBackfillOptions) (*RadioReleaseLinkReport, error) {
	var report *RadioReleaseLinkReport
	err := withReleaseLinkBackfillTx(db, opts.DryRun, func(tx *gorm.DB) error {
		var err error
		report, err = evaluateRadioReleaseLinks(tx, opts.Limit)
		if err != nil || opts.DryRun {
			return err
		}
		return applyRadioReleaseLinkChanges(tx, report)
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// withReleaseLinkBackfillTx runs fn in one REPEATABLE READ transaction, so every
// read sees one snapshot and a row changed by another session after that
// snapshot fails the write instead of being overwritten. A read-only run's
// transaction is READ ONLY: the server rejects any write in it.
func withReleaseLinkBackfillTx(db *gorm.DB, readOnly bool, fn func(tx *gorm.DB) error) error {
	mode := "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ WRITE"
	if readOnly {
		mode = "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY"
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(mode).Error; err != nil {
			return fmt.Errorf("setting transaction mode: %w", err)
		}
		return fn(tx)
	})
}

type linkedRadioPlay struct {
	ID         uint
	ArtistID   *uint
	ArtistName string
	AlbumTitle *string
	ReleaseID  uint
}

func evaluateRadioReleaseLinks(tx *gorm.DB, limit int) (*RadioReleaseLinkReport, error) {
	var plays []linkedRadioPlay
	q := tx.Table("radio_plays").
		Select("id, artist_id, artist_name, album_title, release_id").
		Where("release_id IS NOT NULL").
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(&plays).Error; err != nil {
		return nil, fmt.Errorf("loading linked plays: %w", err)
	}

	titleKeys := make([]string, 0, len(plays))
	seenKeys := make(map[string]bool)
	currentIDs := make([]int64, 0, len(plays))
	seenReleases := make(map[uint]bool)
	for _, p := range plays {
		if key := normalizeName(derefString(p.AlbumTitle)); key != "" && !seenKeys[key] {
			seenKeys[key] = true
			titleKeys = append(titleKeys, key)
		}
		if !seenReleases[p.ReleaseID] {
			seenReleases[p.ReleaseID] = true
			currentIDs = append(currentIDs, int64(p.ReleaseID))
		}
	}

	candidates, err := loadReleaseLinkCandidates(tx, titleKeys)
	if err != nil {
		return nil, err
	}
	creditRows, err := loadReleaseCreditRows(tx, currentIDs)
	if err != nil {
		return nil, err
	}
	currentCredits := make(map[uint][]releaseCreditRow)
	for _, row := range creditRows {
		currentCredits[row.ReleaseID] = append(currentCredits[row.ReleaseID], row)
	}
	adminSet, err := loadAdminSetReleaseLinks(tx)
	if err != nil {
		return nil, err
	}

	report := &RadioReleaseLinkReport{
		Scanned: len(plays),
		Counts:  make(map[RadioReleaseLinkOutcome]int, len(RadioReleaseLinkOutcomes)),
	}
	for _, p := range plays {
		outcome, newID := classifyReleaseLink(p, candidates, currentCredits[p.ReleaseID], adminSet)
		report.Counts[outcome]++
		if !outcome.IsRemoval() && !outcome.IsRelink() {
			continue
		}
		change := RadioReleaseLinkChange{
			PlayID:           p.ID,
			Outcome:          outcome,
			ArtistID:         p.ArtistID,
			ArtistName:       p.ArtistName,
			AlbumTitle:       derefString(p.AlbumTitle),
			CurrentReleaseID: p.ReleaseID,
			NewReleaseID:     newID,
		}
		for _, row := range currentCredits[p.ReleaseID] {
			change.CurrentReleaseArtists = append(change.CurrentReleaseArtists, row.ArtistName)
		}
		report.Changes = append(report.Changes, change)
	}

	if err := fillReleaseTitles(tx, report.Changes); err != nil {
		return nil, err
	}
	return report, nil
}

// classifyReleaseLink returns the outcome for one play and, for a relink, the
// release pickReleaseLink chooses. A link the rule would remove or replace is
// kept when an admin set that release on that play.
func classifyReleaseLink(
	p linkedRadioPlay,
	candidates map[string][]releaseLinkCandidate,
	currentCredits []releaseCreditRow,
	adminSet map[uint]map[uint]bool,
) (RadioReleaseLinkOutcome, *uint) {
	outcome, picked := classifyByRule(p, candidates, currentCredits)
	if (outcome.IsRemoval() || outcome.IsRelink()) && adminSet[p.ID][p.ReleaseID] {
		return ReleaseLinkKeptAdminSet, nil
	}
	return outcome, picked
}

func classifyByRule(
	p linkedRadioPlay,
	candidates map[string][]releaseLinkCandidate,
	currentCredits []releaseCreditRow,
) (RadioReleaseLinkOutcome, *uint) {
	if p.ArtistID != nil {
		for _, row := range currentCredits {
			if row.ArtistID == *p.ArtistID {
				return ReleaseLinkKeptCredited, nil
			}
		}
	}
	picked := pickReleaseLink(p.ArtistID, normalizeName(p.ArtistName), candidates[normalizeName(derefString(p.AlbumTitle))])
	switch {
	case p.ArtistID != nil && picked != nil:
		return ReleaseLinkRelinkedCredited, picked
	case p.ArtistID != nil:
		return ReleaseLinkRemovedNotCredited, nil
	case picked == nil:
		return ReleaseLinkRemovedUnlinkedArtist, nil
	case *picked == p.ReleaseID:
		return ReleaseLinkKeptUniqueTitle, nil
	default:
		return ReleaseLinkRelinkedUniqueTitle, picked
	}
}

// loadAdminSetReleaseLinks returns, per play id, the release ids an admin set
// on it through the link-play endpoint.
func loadAdminSetReleaseLinks(tx *gorm.DB) (map[uint]map[uint]bool, error) {
	var rows []struct {
		PlayID    uint
		ReleaseID uint
	}
	if err := tx.Raw(`
SELECT entity_id AS play_id, (metadata->>'release_id')::bigint AS release_id
FROM audit_logs
WHERE action = ? AND entity_type = 'radio_play'
  AND metadata->>'release_id' ~ '^[0-9]+$'`, adminLinkPlayAuditAction).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading admin-set release links: %w", err)
	}
	set := make(map[uint]map[uint]bool)
	for _, row := range rows {
		if set[row.PlayID] == nil {
			set[row.PlayID] = make(map[uint]bool)
		}
		set[row.PlayID][row.ReleaseID] = true
	}
	return set, nil
}

func fillReleaseTitles(tx *gorm.DB, changes []RadioReleaseLinkChange) error {
	ids := make([]int64, 0, 2*len(changes))
	for _, c := range changes {
		ids = append(ids, int64(c.CurrentReleaseID))
		if c.NewReleaseID != nil {
			ids = append(ids, int64(*c.NewReleaseID))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		ID    uint
		Title string
	}
	if err := tx.Raw(`SELECT id, title FROM releases WHERE id = ANY(?)`, pq.Array(ids)).Scan(&rows).Error; err != nil {
		return fmt.Errorf("loading release titles: %w", err)
	}
	titles := make(map[uint]string, len(rows))
	for _, row := range rows {
		titles[row.ID] = row.Title
	}
	for i := range changes {
		changes[i].CurrentReleaseTitle = titles[changes[i].CurrentReleaseID]
		if changes[i].NewReleaseID != nil {
			changes[i].NewReleaseTitle = titles[*changes[i].NewReleaseID]
		}
	}
	return nil
}

func applyRadioReleaseLinkChanges(tx *gorm.DB, report *RadioReleaseLinkReport) error {
	var clearIDs, clearFrom, relinkIDs, relinkFrom, relinkTo []int64
	for _, c := range report.Changes {
		switch {
		case c.Outcome.IsRemoval():
			clearIDs = append(clearIDs, int64(c.PlayID))
			clearFrom = append(clearFrom, int64(c.CurrentReleaseID))
		case c.Outcome.IsRelink():
			relinkIDs = append(relinkIDs, int64(c.PlayID))
			relinkFrom = append(relinkFrom, int64(c.CurrentReleaseID))
			relinkTo = append(relinkTo, int64(*c.NewReleaseID))
		}
	}

	if len(clearIDs) > 0 {
		res := tx.Exec(`
UPDATE radio_plays AS p SET release_id = NULL
FROM unnest(?::bigint[], ?::bigint[]) AS v(play_id, from_release_id)
WHERE p.id = v.play_id AND p.release_id = v.from_release_id`,
			pq.Array(clearIDs), pq.Array(clearFrom))
		if res.Error != nil {
			return fmt.Errorf("clearing release links: %w", res.Error)
		}
		report.Cleared = res.RowsAffected
	}
	if len(relinkIDs) > 0 {
		res := tx.Exec(`
UPDATE radio_plays AS p SET release_id = v.to_release_id
FROM unnest(?::bigint[], ?::bigint[], ?::bigint[]) AS v(play_id, from_release_id, to_release_id)
WHERE p.id = v.play_id AND p.release_id = v.from_release_id`,
			pq.Array(relinkIDs), pq.Array(relinkFrom), pq.Array(relinkTo))
		if res.Error != nil {
			return fmt.Errorf("relinking releases: %w", res.Error)
		}
		report.Relinked = res.RowsAffected
	}
	return nil
}
