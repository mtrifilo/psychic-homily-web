package catalog

import (
	"fmt"

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

// judgeArtistRelease is the credit rule for a play linked to artistID whose
// current release is current (nil when it has none). kept reports that
// current is credited to the artist. Otherwise credited counts the candidates
// credited to the artist, and picked is the one of them when credited is 1,
// else nil.
func judgeArtistRelease(artistID uint, current *releaseLinkCandidate, candidates []releaseLinkCandidate) (kept bool, credited int, picked *releaseLinkCandidate) {
	if current != nil && current.creditsArtist(artistID) {
		return true, 0, nil
	}
	credited = countCreditedTo(artistID, candidates)
	if credited == 1 {
		picked = pickReleaseLink(&artistID, "", candidates)
	}
	return false, credited, picked
}

// applyReleaseCreditRule applies judgeArtistRelease to each play in playIDs
// that has a linked artist and no admin-chosen release: a credited release is
// kept, the one credited candidate is linked and marked 'matcher', and the
// play is left with no release when none or several candidates are credited.
// It returns how many plays it linked and how many it cleared. Callers hold
// the row locks on playIDs (they run it in the transaction that linked the
// artists), so the rows it reads are the rows it writes.
func applyReleaseCreditRule(tx *gorm.DB, playIDs []int64) (linked, cleared int64, err error) {
	if len(playIDs) == 0 {
		return 0, 0, nil
	}
	var plays []struct {
		ID         uint
		ArtistID   uint
		AlbumTitle *string
		ReleaseID  *uint
	}
	if err := tx.Raw(`
SELECT id, artist_id, album_title, release_id
FROM radio_plays
WHERE id = ANY(?) AND artist_id IS NOT NULL
  AND (release_id IS NULL OR release_link_source IS DISTINCT FROM ?)
ORDER BY id`, pq.Array(playIDs), catalogm.RadioPlayReleaseLinkSourceAdmin).Scan(&plays).Error; err != nil {
		return 0, 0, fmt.Errorf("loading linked plays: %w", err)
	}

	var titleKeys []string
	seenKeys := make(map[string]bool)
	var currentIDs []int64
	for _, p := range plays {
		if key := normalizeName(derefString(p.AlbumTitle)); key != "" && !seenKeys[key] {
			seenKeys[key] = true
			titleKeys = append(titleKeys, key)
		}
		if p.ReleaseID != nil {
			currentIDs = append(currentIDs, int64(*p.ReleaseID))
		}
	}
	candidates, err := loadReleaseLinkCandidates(tx, titleKeys)
	if err != nil {
		return 0, 0, err
	}
	credits, err := loadReleaseCredits(tx, currentIDs)
	if err != nil {
		return 0, 0, err
	}

	var linkIDs, linkTo, clearIDs []int64
	for _, p := range plays {
		var current *releaseLinkCandidate
		if p.ReleaseID != nil {
			current = &releaseLinkCandidate{ReleaseID: *p.ReleaseID, Credits: credits[*p.ReleaseID]}
		}
		kept, _, picked := judgeArtistRelease(p.ArtistID, current, candidates[normalizeName(derefString(p.AlbumTitle))])
		switch {
		case kept:
		case picked != nil:
			linkIDs = append(linkIDs, int64(p.ID))
			linkTo = append(linkTo, int64(picked.ReleaseID))
		case current != nil:
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
			return 0, 0, fmt.Errorf("linking credited releases: %w", res.Error)
		}
		linked = res.RowsAffected
	}
	if len(clearIDs) > 0 {
		res := tx.Exec(`UPDATE radio_plays SET release_id = NULL, release_link_source = NULL WHERE id = ANY(?)`, pq.Array(clearIDs))
		if res.Error != nil {
			return 0, 0, fmt.Errorf("clearing uncredited releases: %w", res.Error)
		}
		cleared = res.RowsAffected
	}
	return linked, cleared, nil
}

// countCreditedTo returns how many candidates credit the artist.
func countCreditedTo(artistID uint, candidates []releaseLinkCandidate) int {
	n := 0
	for _, c := range candidates {
		if c.creditsArtist(artistID) {
			n++
		}
	}
	return n
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
