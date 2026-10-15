package catalog

import (
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// releaseLinkCandidate is a catalog release with every distinct artist
// credited on it (any artist_releases role).
type releaseLinkCandidate struct {
	ReleaseID uint
	Title     string
	Credits   []releaseCredit
}

// releaseCredit is one artist credited on a release.
type releaseCredit struct {
	ArtistID   uint
	ArtistName string
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
//     that release credits exactly one artist, and that artist's name equals
//     the play's artist name under normalizeName.
//
// Anything else returns nil: the play gets no release link.
func pickReleaseLink(artistID *uint, artistName string, candidates []releaseLinkCandidate) *uint {
	if artistID != nil {
		for i := range candidates {
			if candidates[i].creditsArtist(*artistID) {
				return &candidates[i].ReleaseID
			}
		}
		return nil
	}
	if len(candidates) != 1 || len(candidates[0].Credits) != 1 {
		return nil
	}
	nameKey := normalizeName(artistName)
	if nameKey == "" || normalizeName(candidates[0].Credits[0].ArtistName) != nameKey {
		return nil
	}
	return &candidates[0].ReleaseID
}

// releaseCreditRow is one (release, credited artist) row; the artist columns
// are NULL for a release without credits.
type releaseCreditRow struct {
	ReleaseID  uint
	Title      string
	TitleKey   string
	ArtistID   *uint
	ArtistName *string
}

// loadReleaseLinkCandidates returns, per title key, the releases whose
// radio_normalize_name(title) equals that key, each with its credited artists,
// in one query. Keys are normalizeName output; releases are ordered by id.
func loadReleaseLinkCandidates(db *gorm.DB, titleKeys []string) (map[string][]releaseLinkCandidate, error) {
	byKey := make(map[string][]releaseLinkCandidate)
	if len(titleKeys) == 0 {
		return byKey, nil
	}
	var rows []releaseCreditRow
	if err := db.Raw(`
SELECT r.id AS release_id, r.title, radio_normalize_name(r.title) AS title_key,
       c.artist_id, c.artist_name
FROM releases r
LEFT JOIN LATERAL (
  SELECT DISTINCT ar.artist_id, a.name AS artist_name
  FROM artist_releases ar
  JOIN artists a ON a.id = ar.artist_id
  WHERE ar.release_id = r.id
) c ON true
WHERE radio_normalize_name(r.title) = ANY(?)
ORDER BY r.id, c.artist_id`, pq.Array(titleKeys)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading releases by title: %w", err)
	}
	for _, row := range rows {
		list := byKey[row.TitleKey]
		if n := len(list); n == 0 || list[n-1].ReleaseID != row.ReleaseID {
			list = append(list, releaseLinkCandidate{ReleaseID: row.ReleaseID, Title: row.Title})
		}
		if row.ArtistID != nil {
			last := &list[len(list)-1]
			last.Credits = append(last.Credits, releaseCredit{ArtistID: *row.ArtistID, ArtistName: derefString(row.ArtistName)})
		}
		byKey[row.TitleKey] = list
	}
	return byKey, nil
}

// loadReleaseCredits returns the distinct artists credited on each release
// (any role), ordered by artist id.
func loadReleaseCredits(db *gorm.DB, releaseIDs []int64) (map[uint][]releaseCredit, error) {
	credits := make(map[uint][]releaseCredit)
	if len(releaseIDs) == 0 {
		return credits, nil
	}
	var rows []struct {
		ReleaseID  uint
		ArtistID   uint
		ArtistName string
	}
	if err := db.Raw(`
SELECT DISTINCT ar.release_id, ar.artist_id, a.name AS artist_name
FROM artist_releases ar
JOIN artists a ON a.id = ar.artist_id
WHERE ar.release_id = ANY(?)
ORDER BY ar.release_id, ar.artist_id`, pq.Array(releaseIDs)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading release credits: %w", err)
	}
	for _, row := range rows {
		credits[row.ReleaseID] = append(credits[row.ReleaseID], releaseCredit{ArtistID: row.ArtistID, ArtistName: row.ArtistName})
	}
	return credits, nil
}
