package admin

import (
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// bandcampEmbedRefusedCandidateSQL is a COARSE pre-filter, not the answer:
// every row that holds a value at all. utils.IsValidBandcampEmbedURL decides,
// so the count, the list and the reason are the SAME predicate every write path
// gates on rather than a SQL restatement of it that can drift from the rule.
//
// A restatement is not merely redundant here, it cannot be correct: the rule
// judges the path as WRITTEN, refuses surrounding whitespace, and rejects dot
// segments and backslashes in either spelling. A regular expression that agreed
// with all of that would be the harder thing to keep in step, not the easier.
//
// Applies to the artists table aliased `a`.
const bandcampEmbedRefusedCandidateSQL = `a.bandcamp_embed_url IS NOT NULL`

// bandcampEmbedRefusedFindings returns every artist whose stored Bandcamp embed
// URL the release-page gate refuses, newest first.
//
// Whole set, not a page: the count and the list are then the same answer.
// Callers page the slice.
func (s *DataQualityService) bandcampEmbedRefusedFindings() ([]*contracts.DataQualityItem, error) {
	type row struct {
		ID        uint
		Name      string
		Slug      string
		EmbedURL  string
		ShowCount int
	}
	var rows []row
	// show_count matches the other artist gap categories, so the item shape is
	// consistent across them.
	err := s.db.Raw(`
		SELECT a.id, a.name, COALESCE(a.slug, '') AS slug,
		       a.bandcamp_embed_url AS embed_url,
		       COUNT(sa.show_id) AS show_count
		FROM artists a
		LEFT JOIN show_artists sa ON sa.artist_id = a.id
		LEFT JOIN shows s ON s.id = sa.show_id AND s.status = 'approved'
		WHERE ` + bandcampEmbedRefusedCandidateSQL + `
		GROUP BY a.id
		ORDER BY a.id DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]*contracts.DataQualityItem, 0)
	for _, r := range rows {
		if utils.IsValidBandcampEmbedURL(r.EmbedURL) {
			continue
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       r.Slug,
			// The shape of the refusal, never the value: it is a
			// contributor-written URL, and a list that reprinted it would put a
			// live hostile link in front of whoever opened the page.
			Reason:    bandcampEmbedRefusal(r.EmbedURL),
			ShowCount: r.ShowCount,
		})
	}
	return items, nil
}

// getBandcampEmbedRefused pages the findings for the category.
func (s *DataQualityService) getBandcampEmbedRefused(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	findings, err := s.bandcampEmbedRefusedFindings()
	if err != nil {
		return nil, 0, err
	}
	return pageItems(findings, limit, offset), int64(len(findings)), nil
}

// bandcampEmbedRefusal names a refused value in the two shapes an admin acts on
// differently: a foreign or non-https host is a link wearing a Bandcamp label
// and pointing somewhere else, while an on-platform value is a legacy row that
// only needs a better URL.
//
// Both arms read existing named predicates rather than restating any part of
// either rule. utils.IsResolvableBandcampURL is the looser host-only floor the
// embed resolver applies, so it is exactly the line between the two sentences.
//
// Neither sentence says WHICH rule the value broke. The host is the only part
// either predicate isolates, and guessing at the rest would name the wrong one:
// a stored "https://x.bandcamp.com/album/y " clears the host floor with an
// album path and is still refused, for the trailing space.
func bandcampEmbedRefusal(rawURL string) string {
	if utils.IsResolvableBandcampURL(rawURL) {
		return "On a Bandcamp host, but not a valid release URL"
	}
	return "Not an https URL on a Bandcamp host"
}
