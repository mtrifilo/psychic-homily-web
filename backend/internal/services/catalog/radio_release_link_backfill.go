package catalog

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// RadioReleaseLinkOutcome classifies one existing radio_plays.release_id under
// pickReleaseLink.
type RadioReleaseLinkOutcome string

const (
	// RadioReleaseLinkKeptAdmin: an admin chose the release
	// (release_link_source 'admin'); the backfill never judges it.
	RadioReleaseLinkKeptAdmin RadioReleaseLinkOutcome = "kept_admin"
	// RadioReleaseLinkKeptCredited: the play has a linked artist and the release
	// is credited to it.
	RadioReleaseLinkKeptCredited RadioReleaseLinkOutcome = "kept_credited"
	// RadioReleaseLinkKeptUniqueTitle: the play has no linked artist and the
	// release is the one pickReleaseLink chooses for it.
	RadioReleaseLinkKeptUniqueTitle RadioReleaseLinkOutcome = "kept_unique_title"
	// RadioReleaseLinkRemovedNotCredited: the play has a linked artist, the
	// release is not credited to it, and no release with the title is.
	RadioReleaseLinkRemovedNotCredited RadioReleaseLinkOutcome = "removed_not_credited"
	// RadioReleaseLinkRemovedSeveralCredited: the play has a linked artist, the
	// release is not credited to it, and two or more releases with the title
	// are, so no single release replaces it.
	RadioReleaseLinkRemovedSeveralCredited RadioReleaseLinkOutcome = "removed_several_credited"
	// RadioReleaseLinkRemovedUnlinkedArtist: the play has no linked artist and
	// pickReleaseLink chooses no release for it.
	RadioReleaseLinkRemovedUnlinkedArtist RadioReleaseLinkOutcome = "removed_unlinked_artist"
	// RadioReleaseLinkRelinkedCredited: the play has a linked artist, the
	// release is not credited to it, and exactly one release with the title is.
	RadioReleaseLinkRelinkedCredited RadioReleaseLinkOutcome = "relinked_credited"
	// RadioReleaseLinkRelinkedUniqueTitle: the play has no linked artist and
	// pickReleaseLink chooses a different release for it.
	RadioReleaseLinkRelinkedUniqueTitle RadioReleaseLinkOutcome = "relinked_unique_title"
)

// RadioReleaseLinkOutcomes lists every outcome in report order.
var RadioReleaseLinkOutcomes = []RadioReleaseLinkOutcome{
	RadioReleaseLinkKeptAdmin,
	RadioReleaseLinkKeptCredited,
	RadioReleaseLinkKeptUniqueTitle,
	RadioReleaseLinkRemovedNotCredited,
	RadioReleaseLinkRemovedSeveralCredited,
	RadioReleaseLinkRemovedUnlinkedArtist,
	RadioReleaseLinkRelinkedCredited,
	RadioReleaseLinkRelinkedUniqueTitle,
}

// IsRemoval reports whether the outcome clears the play's release link.
func (o RadioReleaseLinkOutcome) IsRemoval() bool {
	switch o {
	case RadioReleaseLinkRemovedNotCredited, RadioReleaseLinkRemovedSeveralCredited, RadioReleaseLinkRemovedUnlinkedArtist:
		return true
	}
	return false
}

// IsRelink reports whether the outcome points the play at another release.
func (o RadioReleaseLinkOutcome) IsRelink() bool {
	return o == RadioReleaseLinkRelinkedCredited || o == RadioReleaseLinkRelinkedUniqueTitle
}

// ChangesLink reports whether the outcome removes or replaces the link.
func (o RadioReleaseLinkOutcome) ChangesLink() bool {
	return o.IsRemoval() || o.IsRelink()
}

// RadioReleaseLinkChange is one play whose release link the backfill removes
// or replaces.
type RadioReleaseLinkChange struct {
	PlayID     uint
	Outcome    RadioReleaseLinkOutcome
	ArtistID   *uint
	ArtistName string
	// LinkedArtistName is the catalog name of ArtistID, empty when nil.
	LinkedArtistName      string
	AlbumTitle            string
	CurrentReleaseID      uint
	CurrentReleaseTitle   string
	CurrentReleaseArtists []string
	// NewReleaseID and NewReleaseTitle are set for relinks only.
	NewReleaseID    *uint
	NewReleaseTitle string
	// AdminSet: an audit_logs link_radio_play row records an admin setting
	// this play's current release, though its release_link_source is not
	// 'admin' (a play marked 'admin' is never changed). The flag only marks
	// the change for review.
	AdminSet bool
}

// RadioReleaseLinkReport is the result of one backfill run. Changes holds every
// removal and relink, ordered by play id.
type RadioReleaseLinkReport struct {
	Scanned int
	Counts  map[RadioReleaseLinkOutcome]int
	// SourceCounts counts the scanned plays by release_link_source; the empty
	// key counts plays whose release link carries no source.
	SourceCounts map[string]int
	Changes      []RadioReleaseLinkChange
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
	// BeforeApply, when set on a live run, receives the evaluated report
	// inside the transaction before any write; an error aborts the run with
	// nothing written.
	BeforeApply func(*RadioReleaseLinkReport) error
	// txProbe, when set, runs first inside the run's transaction.
	txProbe func(tx *gorm.DB) error
}

// BackfillRadioReleaseLinks judges every play's existing release_id and,
// unless DryRun, clears the links it refuses and relinks the rest it can,
// touching only release_id and release_link_source. Its judgement differs
// from the matcher, which only chooses a release for a play it is matching:
//
//   - A release an admin chose (release_link_source 'admin') is kept and
//     never judged.
//   - A play with a linked artist keeps any release credited to that artist,
//     whatever the release's title; otherwise it relinks only when exactly one
//     release with the play's title credits the artist, and is cleared when
//     none or several do (the matcher takes the lowest-id credited release).
//   - A play without a linked artist keeps or gets the release pickReleaseLink
//     chooses, and is cleared when it chooses none.
//
// Evaluation and writes share one REPEATABLE READ transaction, so every play
// is judged as of one snapshot. If another session changes a play this run
// updates, Postgres rejects the update with a serialization failure (SQLSTATE
// 40001) and the whole run rolls back; re-running is safe. A commit that fails
// returns ErrRadioReleaseLinkCommitUnknown: the server may or may not have
// applied the changes.
func BackfillRadioReleaseLinks(db *gorm.DB, opts RadioReleaseLinkBackfillOptions) (*RadioReleaseLinkReport, error) {
	var report *RadioReleaseLinkReport
	bodySucceeded := false
	err := withReleaseLinkBackfillTx(db, opts.DryRun, func(tx *gorm.DB) error {
		if opts.txProbe != nil {
			if err := opts.txProbe(tx); err != nil {
				return err
			}
		}
		var err error
		report, err = evaluateRadioReleaseLinks(tx, opts.Limit)
		if err != nil || opts.DryRun {
			bodySucceeded = err == nil
			return err
		}
		if opts.BeforeApply != nil {
			if err := opts.BeforeApply(report); err != nil {
				return err
			}
		}
		if err := applyRadioReleaseLinkChanges(tx, report); err != nil {
			return err
		}
		bodySucceeded = true
		return nil
	})
	if err != nil {
		if bodySucceeded && !opts.DryRun {
			return nil, fmt.Errorf("%w: %w", ErrRadioReleaseLinkCommitUnknown, err)
		}
		return nil, err
	}
	return report, nil
}

// ErrRadioReleaseLinkCommitUnknown wraps a live run's failed COMMIT: every
// update succeeded inside the transaction, and whether the server applied
// them is unknown to the client.
var ErrRadioReleaseLinkCommitUnknown = errors.New("commit failed; whether the changes were applied is unknown")

// withReleaseLinkBackfillTx runs fn in one REPEATABLE READ transaction. A
// read-only run's transaction is READ ONLY: the server rejects any write in it.
func withReleaseLinkBackfillTx(db *gorm.DB, readOnly bool, fn func(tx *gorm.DB) error) error {
	return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: readOnly})
}

// playWithRelease is a radio play that carries a release_id.
type playWithRelease struct {
	ID                  uint
	ArtistID            *uint
	ArtistName          string
	LinkedArtistName    string
	AlbumTitle          *string
	ReleaseID           uint
	ReleaseLinkSource   string
	CurrentReleaseTitle string
	titleKey            string
}

func evaluateRadioReleaseLinks(tx *gorm.DB, limit int) (*RadioReleaseLinkReport, error) {
	var plays []playWithRelease
	q := tx.Table("radio_plays AS p").
		Select(`p.id, p.artist_id, p.artist_name, COALESCE(a.name, '') AS linked_artist_name,
			p.album_title, p.release_id, COALESCE(p.release_link_source, '') AS release_link_source,
			COALESCE(r.title, '') AS current_release_title`).
		Joins("LEFT JOIN artists a ON a.id = p.artist_id").
		Joins("LEFT JOIN releases r ON r.id = p.release_id").
		Where("p.release_id IS NOT NULL").
		Order("p.id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(&plays).Error; err != nil {
		return nil, fmt.Errorf("loading plays with a release: %w", err)
	}

	titleKeys := make([]string, len(plays))
	currentIDs := make([]uint, len(plays))
	for i := range plays {
		p := &plays[i]
		p.titleKey = normalizeName(derefString(p.AlbumTitle))
		titleKeys[i] = p.titleKey
		currentIDs[i] = p.ReleaseID
	}
	candidates, currentCredits, err := loadReleaseLinkInputs(tx, titleKeys, currentIDs)
	if err != nil {
		return nil, err
	}
	adminSet, err := loadAdminSetReleaseLinks(tx)
	if err != nil {
		return nil, err
	}

	report := &RadioReleaseLinkReport{
		Scanned:      len(plays),
		Counts:       make(map[RadioReleaseLinkOutcome]int, len(RadioReleaseLinkOutcomes)),
		SourceCounts: make(map[string]int),
	}
	for _, p := range plays {
		report.SourceCounts[p.ReleaseLinkSource]++
		current := releaseLinkCandidate{ReleaseID: p.ReleaseID, Title: p.CurrentReleaseTitle, Credits: currentCredits[p.ReleaseID]}
		outcome, picked := classifyReleaseLink(p, current, candidates[p.titleKey])
		report.Counts[outcome]++
		if !outcome.ChangesLink() {
			continue
		}
		change := RadioReleaseLinkChange{
			PlayID:              p.ID,
			Outcome:             outcome,
			ArtistID:            p.ArtistID,
			ArtistName:          p.ArtistName,
			LinkedArtistName:    p.LinkedArtistName,
			AlbumTitle:          derefString(p.AlbumTitle),
			CurrentReleaseID:    p.ReleaseID,
			CurrentReleaseTitle: p.CurrentReleaseTitle,
			AdminSet:            adminSet[p.ID][p.ReleaseID],
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
// release it moves to. An admin-chosen release is kept unjudged; a linked
// artist keeps any release credited to it.
func classifyReleaseLink(p playWithRelease, current releaseLinkCandidate, candidates []releaseLinkCandidate) (RadioReleaseLinkOutcome, *releaseLinkCandidate) {
	if p.ReleaseLinkSource == catalogm.RadioPlayReleaseLinkSourceAdmin {
		return RadioReleaseLinkKeptAdmin, nil
	}
	if p.ArtistID != nil {
		return classifyArtistRelease(*p.ArtistID, &current, candidates)
	}
	picked := pickReleaseLink(nil, p.ArtistName, candidates)
	switch {
	case picked == nil:
		return RadioReleaseLinkRemovedUnlinkedArtist, nil
	case picked.ReleaseID == p.ReleaseID:
		return RadioReleaseLinkKeptUniqueTitle, nil
	default:
		return RadioReleaseLinkRelinkedUniqueTitle, picked
	}
}

// loadAdminSetReleaseLinks returns, per play id, the release ids that
// audit_logs records an admin setting on it through the link-play endpoint.
// Audit writes are best effort, so a missing row does not prove a link was
// machine-made.
func loadAdminSetReleaseLinks(tx *gorm.DB) (map[uint]map[uint]bool, error) {
	var rows []struct {
		PlayID    uint
		ReleaseID uint
	}
	if err := tx.Raw(`
SELECT entity_id AS play_id, (metadata->>@key)::bigint AS release_id
FROM audit_logs
WHERE action = @action AND entity_type = @entity
  AND metadata->>@key ~ '^[0-9]+$'`, map[string]interface{}{
		"key":    contracts.AuditMetaReleaseID,
		"action": contracts.AuditActionLinkRadioPlay,
		"entity": contracts.AuditEntityRadioPlay,
	}).Scan(&rows).Error; err != nil {
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

// applyRadioReleaseLinkChanges writes the report's removals and relinks,
// marking each relink 'backfill' and clearing the source with each removal.
// Each UPDATE matches a play only while its release_id is still the evaluated
// one and no admin chose it, so a caller running it outside the evaluation
// snapshot never overwrites a newer or an admin link.
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
UPDATE radio_plays AS p SET release_id = NULL, release_link_source = NULL
FROM unnest(?::bigint[], ?::bigint[]) AS v(play_id, from_release_id)
WHERE p.id = v.play_id AND p.release_id = v.from_release_id
  AND p.release_link_source IS DISTINCT FROM ?`,
			pq.Array(clearIDs), pq.Array(clearFrom), catalogm.RadioPlayReleaseLinkSourceAdmin)
		if res.Error != nil {
			return fmt.Errorf("clearing release links: %w", res.Error)
		}
		report.Cleared = res.RowsAffected
	}
	if len(relinkIDs) > 0 {
		res := tx.Exec(`
UPDATE radio_plays AS p SET release_id = v.to_release_id, release_link_source = ?
FROM unnest(?::bigint[], ?::bigint[], ?::bigint[]) AS v(play_id, from_release_id, to_release_id)
WHERE p.id = v.play_id AND p.release_id = v.from_release_id
  AND p.release_link_source IS DISTINCT FROM ?`,
			catalogm.RadioPlayReleaseLinkSourceBackfill, pq.Array(relinkIDs), pq.Array(relinkFrom), pq.Array(relinkTo),
			catalogm.RadioPlayReleaseLinkSourceAdmin)
		if res.Error != nil {
			return fmt.Errorf("relinking releases: %w", res.Error)
		}
		report.Relinked = res.RowsAffected
	}
	return nil
}
