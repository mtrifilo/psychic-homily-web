package catalog

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
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
	// ReleaseLinkKeptAdminSet: the rule would remove or replace the link, but an
	// admin set this release on this play through the link-play endpoint.
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

func (o RadioReleaseLinkOutcome) isRelink() bool {
	return o == ReleaseLinkRelinkedCredited || o == ReleaseLinkRelinkedUniqueTitle
}

// ChangesLink reports whether the outcome removes or replaces the link.
func (o RadioReleaseLinkOutcome) ChangesLink() bool {
	return o.IsRemoval() || o.isRelink()
}

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
	return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: readOnly})
}

type linkedRadioPlay struct {
	ID                  uint
	ArtistID            *uint
	ArtistName          string
	AlbumTitle          *string
	ReleaseID           uint
	CurrentReleaseTitle string
	titleKey            string
}

func evaluateRadioReleaseLinks(tx *gorm.DB, limit int) (*RadioReleaseLinkReport, error) {
	var plays []linkedRadioPlay
	q := tx.Table("radio_plays AS p").
		Select("p.id, p.artist_id, p.artist_name, p.album_title, p.release_id, COALESCE(r.title, '') AS current_release_title").
		Joins("LEFT JOIN releases r ON r.id = p.release_id").
		Where("p.release_id IS NOT NULL").
		Order("p.id")
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
	for i := range plays {
		p := &plays[i]
		p.titleKey = normalizeName(derefString(p.AlbumTitle))
		if p.titleKey != "" && !seenKeys[p.titleKey] {
			seenKeys[p.titleKey] = true
			titleKeys = append(titleKeys, p.titleKey)
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
	currentCredits, err := loadReleaseCredits(tx, currentIDs)
	if err != nil {
		return nil, err
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
		current := releaseLinkCandidate{ReleaseID: p.ReleaseID, Title: p.CurrentReleaseTitle, Credits: currentCredits[p.ReleaseID]}
		outcome, picked := classifyReleaseLink(p, current, candidates[p.titleKey], adminSet[p.ID][p.ReleaseID])
		report.Counts[outcome]++
		if !outcome.ChangesLink() {
			continue
		}
		change := RadioReleaseLinkChange{
			PlayID:              p.ID,
			Outcome:             outcome,
			ArtistID:            p.ArtistID,
			ArtistName:          p.ArtistName,
			AlbumTitle:          derefString(p.AlbumTitle),
			CurrentReleaseID:    p.ReleaseID,
			CurrentReleaseTitle: p.CurrentReleaseTitle,
		}
		for _, credit := range current.Credits {
			change.CurrentReleaseArtists = append(change.CurrentReleaseArtists, credit.ArtistName)
		}
		if picked != nil {
			change.NewReleaseID = &picked.ReleaseID
			change.NewReleaseTitle = picked.Title
		}
		report.Changes = append(report.Changes, change)
	}
	return report, nil
}

// classifyReleaseLink returns the outcome for one play and, for a relink, the
// release pickReleaseLink chooses. A linked artist keeps any release credited
// to it; a link the rule would remove or replace is kept when adminSet.
func classifyReleaseLink(
	p linkedRadioPlay,
	current releaseLinkCandidate,
	candidates []releaseLinkCandidate,
	adminSet bool,
) (RadioReleaseLinkOutcome, *releaseLinkCandidate) {
	if p.ArtistID != nil && current.creditsArtist(*p.ArtistID) {
		return ReleaseLinkKeptCredited, nil
	}
	var picked *releaseLinkCandidate
	if id := pickReleaseLink(p.ArtistID, p.ArtistName, candidates); id != nil {
		for i := range candidates {
			if candidates[i].ReleaseID == *id {
				picked = &candidates[i]
			}
		}
	}
	if p.ArtistID == nil && picked != nil && picked.ReleaseID == p.ReleaseID {
		return ReleaseLinkKeptUniqueTitle, nil
	}
	if adminSet {
		return ReleaseLinkKeptAdminSet, nil
	}
	switch {
	case picked != nil && p.ArtistID != nil:
		return ReleaseLinkRelinkedCredited, picked
	case picked != nil:
		return ReleaseLinkRelinkedUniqueTitle, picked
	case p.ArtistID != nil:
		return ReleaseLinkRemovedNotCredited, nil
	default:
		return ReleaseLinkRemovedUnlinkedArtist, nil
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
  AND metadata->>'release_id' ~ '^[0-9]+$'`, contracts.AuditActionLinkRadioPlay).Scan(&rows).Error; err != nil {
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

func applyRadioReleaseLinkChanges(tx *gorm.DB, report *RadioReleaseLinkReport) error {
	var clearIDs, clearFrom, relinkIDs, relinkFrom, relinkTo []int64
	for _, c := range report.Changes {
		switch {
		case c.Outcome.IsRemoval():
			clearIDs = append(clearIDs, int64(c.PlayID))
			clearFrom = append(clearFrom, int64(c.CurrentReleaseID))
		case c.Outcome.isRelink():
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
