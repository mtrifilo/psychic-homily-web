package catalog

import (
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// releaseLinkCandidate is a catalog release whose normalized title equals a
// play's album title key, with every distinct artist credited on it.
type releaseLinkCandidate struct {
	ReleaseID uint
	Credits   []releaseCredit
}

// releaseCredit is one artist credited on a release (any artist_releases
// role), with the artist name in normalizeName form.
type releaseCredit struct {
	ArtistID uint
	NameKey  string
}

func (c releaseLinkCandidate) creditsArtist(artistID uint) bool {
	for _, credit := range c.Credits {
		if credit.ArtistID == artistID {
			return true
		}
	}
	return false
}

// pickReleaseLink is the release-link rule for a radio play. candidates are
// every release whose normalized title equals the play's album title key,
// ordered by release id.
//
//   - A play with a linked artist links the lowest-id candidate credited to
//     that artist.
//   - A play without one links only when exactly one release carries the title,
//     that release credits exactly one artist, and that artist's name key equals
//     the play's artist name key.
//
// Anything else returns nil: the play gets no release link.
func pickReleaseLink(artistID *uint, artistNameKey string, candidates []releaseLinkCandidate) *uint {
	if artistID != nil {
		for i := range candidates {
			if candidates[i].creditsArtist(*artistID) {
				return &candidates[i].ReleaseID
			}
		}
		return nil
	}
	if artistNameKey == "" || len(candidates) != 1 {
		return nil
	}
	only := candidates[0]
	if len(only.Credits) != 1 || only.Credits[0].NameKey != artistNameKey {
		return nil
	}
	return &only.ReleaseID
}

// loadReleaseLinkCandidates returns, per title key, the releases whose
// radio_normalize_name(title) equals that key, each with its credited artists.
// Keys are normalizeName output; releases are ordered by id within a key.
func loadReleaseLinkCandidates(db *gorm.DB, titleKeys []string) (map[string][]releaseLinkCandidate, error) {
	byKey := make(map[string][]releaseLinkCandidate)
	if len(titleKeys) == 0 {
		return byKey, nil
	}

	var releases []struct {
		ID       uint
		TitleKey string
	}
	if err := db.Raw(`
SELECT id, radio_normalize_name(title) AS title_key
FROM releases
WHERE radio_normalize_name(title) = ANY(?)
ORDER BY id`, pq.Array(titleKeys)).Scan(&releases).Error; err != nil {
		return nil, fmt.Errorf("loading releases by title: %w", err)
	}
	if len(releases) == 0 {
		return byKey, nil
	}

	releaseIDs := make([]int64, len(releases))
	for i, r := range releases {
		releaseIDs[i] = int64(r.ID)
	}
	credits, err := loadReleaseCredits(db, releaseIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range releases {
		byKey[r.TitleKey] = append(byKey[r.TitleKey], releaseLinkCandidate{
			ReleaseID: r.ID,
			Credits:   credits[r.ID],
		})
	}
	return byKey, nil
}

// releaseCreditRow is one credited artist of a release, with the artist's
// display name.
type releaseCreditRow struct {
	ReleaseID  uint
	ArtistID   uint
	ArtistName string
}

// loadReleaseCreditRows returns the distinct artists credited on each release
// (any role), ordered by release id then artist id.
func loadReleaseCreditRows(db *gorm.DB, releaseIDs []int64) ([]releaseCreditRow, error) {
	var rows []releaseCreditRow
	if len(releaseIDs) == 0 {
		return rows, nil
	}
	if err := db.Raw(`
SELECT DISTINCT ar.release_id, ar.artist_id, a.name AS artist_name
FROM artist_releases ar
JOIN artists a ON a.id = ar.artist_id
WHERE ar.release_id = ANY(?)
ORDER BY ar.release_id, ar.artist_id`, pq.Array(releaseIDs)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading release credits: %w", err)
	}
	return rows, nil
}

// loadReleaseCredits returns the distinct artists credited on each release.
func loadReleaseCredits(db *gorm.DB, releaseIDs []int64) (map[uint][]releaseCredit, error) {
	rows, err := loadReleaseCreditRows(db, releaseIDs)
	if err != nil {
		return nil, err
	}
	credits := make(map[uint][]releaseCredit)
	for _, row := range rows {
		credits[row.ReleaseID] = append(credits[row.ReleaseID], releaseCredit{
			ArtistID: row.ArtistID,
			NameKey:  normalizeName(row.ArtistName),
		})
	}
	return credits, nil
}
