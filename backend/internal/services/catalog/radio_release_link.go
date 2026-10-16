package catalog

import (
	"fmt"
	"log/slog"

	"github.com/lib/pq"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
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
func pickReleaseLink(artistID *uint, artistName string, candidates []releaseLinkCandidate) *releaseLinkCandidate {
	if artistID != nil {
		for i := range candidates {
			if candidates[i].creditsArtist(*artistID) {
				return &candidates[i]
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
	return &candidates[0]
}

// classifyArtistRelease is the credit rule for a play linked to artistID
// whose current release is current (nil when it has none). A current release
// credited to the artist is kept; otherwise the play relinks to the one
// candidate credited to the artist, and is left with no release when none or
// several are. It returns the outcome and, for a relink, the release.
func classifyArtistRelease(artistID uint, current *releaseLinkCandidate, candidates []releaseLinkCandidate) (RadioReleaseLinkOutcome, *releaseLinkCandidate) {
	if current != nil && current.creditsArtist(artistID) {
		return RadioReleaseLinkKeptCredited, nil
	}
	var picked *releaseLinkCandidate
	for i := range candidates {
		if !candidates[i].creditsArtist(artistID) {
			continue
		}
		if picked != nil {
			return RadioReleaseLinkRemovedSeveralCredited, nil
		}
		picked = &candidates[i]
	}
	if picked == nil {
		return RadioReleaseLinkRemovedNotCredited, nil
	}
	return RadioReleaseLinkRelinkedCredited, picked
}

// linkedPlay is a play an artist-linking UPDATE returns, with the columns the
// credit rule reads.
type linkedPlay struct {
	ID                uint
	ArtistID          uint
	AlbumTitle        *string
	ReleaseID         *uint
	ReleaseLinkSource *string
}

// linkArtistsWithCreditRule runs query, an UPDATE that links artists to plays
// and RETURNs each linked play's linkedPlay columns, then applies
// classifyArtistRelease to those plays in the same transaction: a credited
// release is kept, the one credited candidate is linked and marked 'matcher',
// and the play is left with no release when none or several candidates are
// credited. A release an admin chose is never changed. The UPDATE holds the
// row locks on every play it returns until tx ends, so the rows judged are the
// rows written. It returns how many plays the query linked, and how many
// releases it linked and cleared.
func linkArtistsWithCreditRule(tx *gorm.DB, query string, args ...interface{}) (linkedPlays, linkedReleases, clearedReleases int64, err error) {
	var plays []linkedPlay
	if err := tx.Raw(query, args...).Scan(&plays).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("linking artists: %w", err)
	}
	titleKeys := make([]string, len(plays))
	var currentIDs []uint
	for i, p := range plays {
		titleKeys[i] = normalizeName(derefString(p.AlbumTitle))
		if p.ReleaseID != nil {
			currentIDs = append(currentIDs, *p.ReleaseID)
		}
	}
	candidates, credits, err := loadReleaseLinkInputs(tx, titleKeys, currentIDs)
	if err != nil {
		return 0, 0, 0, err
	}

	var linkIDs, linkTo, clearIDs []int64
	for i, p := range plays {
		var current *releaseLinkCandidate
		if p.ReleaseID != nil {
			if derefString(p.ReleaseLinkSource) == catalogm.RadioPlayReleaseLinkSourceAdmin {
				continue
			}
			current = &releaseLinkCandidate{ReleaseID: *p.ReleaseID, Credits: credits[*p.ReleaseID]}
		}
		outcome, picked := classifyArtistRelease(p.ArtistID, current, candidates[titleKeys[i]])
		switch {
		case outcome.IsRelink():
			var fromReleaseID any
			if current != nil {
				fromReleaseID = current.ReleaseID
			}
			slog.Info("radio bulk link: linked the release credited to the play's artist",
				"play_id", p.ID, "artist_id", p.ArtistID, "from_release_id", fromReleaseID, "to_release_id", picked.ReleaseID)
			linkIDs = append(linkIDs, int64(p.ID))
			linkTo = append(linkTo, int64(picked.ReleaseID))
		case outcome.IsRemoval() && current != nil:
			slog.Info("radio bulk link: cleared release not credited to the play's artist",
				"play_id", p.ID, "artist_id", p.ArtistID, "release_id", *p.ReleaseID, "outcome", outcome)
			clearIDs = append(clearIDs, int64(p.ID))
		}
	}

	if len(linkIDs) > 0 {
		res := tx.Exec(`
UPDATE radio_plays AS p SET release_id = v.release_id, release_link_source = ?
FROM unnest(?::bigint[], ?::bigint[]) AS v(play_id, release_id)
WHERE p.id = v.play_id`,
			catalogm.RadioPlayReleaseLinkSourceMatcher, pq.Array(linkIDs), pq.Array(linkTo))
		if res.Error != nil {
			return 0, 0, 0, fmt.Errorf("linking credited releases: %w", res.Error)
		}
		linkedReleases = res.RowsAffected
	}
	if len(clearIDs) > 0 {
		res := tx.Exec(`UPDATE radio_plays SET release_id = NULL, release_link_source = NULL WHERE id = ANY(?)`, pq.Array(clearIDs))
		if res.Error != nil {
			return 0, 0, 0, fmt.Errorf("clearing uncredited releases: %w", res.Error)
		}
		clearedReleases = res.RowsAffected
	}
	return int64(len(plays)), linkedReleases, clearedReleases, nil
}

// loadReleaseLinkInputs loads the candidates for every non-empty title key and
// the credits of every release id; duplicates in either list are queried once.
func loadReleaseLinkInputs(db *gorm.DB, titleKeys []string, releaseIDs []uint) (map[string][]releaseLinkCandidate, map[uint][]releaseCredit, error) {
	keys := make([]string, 0, len(titleKeys))
	seenKeys := make(map[string]bool, len(titleKeys))
	for _, key := range titleKeys {
		if key != "" && !seenKeys[key] {
			seenKeys[key] = true
			keys = append(keys, key)
		}
	}
	ids := make([]int64, 0, len(releaseIDs))
	seenIDs := make(map[uint]bool, len(releaseIDs))
	for _, id := range releaseIDs {
		if !seenIDs[id] {
			seenIDs[id] = true
			ids = append(ids, int64(id))
		}
	}
	candidates, err := loadReleaseLinkCandidates(db, keys)
	if err != nil {
		return nil, nil, err
	}
	credits, err := loadReleaseCredits(db, ids)
	if err != nil {
		return nil, nil, err
	}
	return candidates, credits, nil
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
