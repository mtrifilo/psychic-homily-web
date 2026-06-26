package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func rg(mbid, title, date string, artists ...string) MBReleaseGroupCandidate {
	return MBReleaseGroupCandidate{MBID: mbid, Title: title, FirstReleaseDate: date, ArtistNames: artists}
}

func TestPickStrictReleaseGroup_ExactSingleMatch(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "2003", "Sleep")}
	mbid, qualifying := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid)
	assert.Equal(t, 1, qualifying)
}

func TestPickStrictReleaseGroup_TitleMismatch(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Holy Mountain", "1992", "Sleep")}
	mbid, qualifying := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "", mbid)
	assert.Equal(t, 0, qualifying)
}

func TestPickStrictReleaseGroup_ArtistMismatch(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "2003", "Not Sleep")}
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "", mbid)
}

func TestPickStrictReleaseGroup_YearOutOfTolerance(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "1999", "Sleep")}
	mbid, qualifying := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "", mbid, "4-year gap exceeds the one-year tolerance")
	assert.Equal(t, 0, qualifying)
}

func TestPickStrictReleaseGroup_YearWithinTolerance(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "2004", "Sleep")}
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid, "one year of drift is allowed")
}

func TestPickStrictReleaseGroup_UnknownYearNotRejected(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "", "Sleep")}
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid, "a candidate with no date is never rejected on year")
}

func TestPickStrictReleaseGroup_AmbiguousDistinctReleaseGroups(t *testing.T) {
	cands := []MBReleaseGroupCandidate{
		rg("rg-1", "Dopesmoker", "2003", "Sleep"),
		rg("rg-2", "Dopesmoker", "2003", "Sleep"),
	}
	mbid, qualifying := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "", mbid, "two distinct release-groups the year can't disambiguate → skip")
	assert.Equal(t, 2, qualifying)
}

func TestPickStrictReleaseGroup_DuplicateRowsSameMBID(t *testing.T) {
	cands := []MBReleaseGroupCandidate{
		rg("rg-1", "Dopesmoker", "2003", "Sleep"),
		rg("rg-1", "Dopesmoker", "2003", "Sleep"),
	}
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid, "same mbid twice is not ambiguous")
}

func TestPickStrictReleaseGroup_YearDisambiguatesDistinct(t *testing.T) {
	cands := []MBReleaseGroupCandidate{
		rg("rg-1", "Dopesmoker", "2003", "Sleep"),
		rg("rg-2", "Dopesmoker", "2004", "Sleep"),
	}
	// Both pass the ±1 gate, but only rg-1 matches the year EXACTLY.
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid)
}

func TestPickStrictReleaseGroup_MatchesCanonicalArtistName(t *testing.T) {
	// Credited name differs ("Sleep feat. X"); canonical "Sleep" still matches.
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Dopesmoker", "2003", "Sleep feat. Guest", "Sleep")}
	mbid, _ := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "rg-1", mbid)
}

func TestPickStrictReleaseGroup_NormalizesDiacriticsAndPunctuation(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("rg-1", "Café Tacvba", "1992", "Café Tacvba")}
	mbid, _ := pickStrictReleaseGroup(cands, "cafe tacvba", "cafe tacvba", intPtr(1992))
	assert.Equal(t, "rg-1", mbid)
}

func TestPickStrictReleaseGroup_BlankMBIDSkipped(t *testing.T) {
	cands := []MBReleaseGroupCandidate{rg("  ", "Dopesmoker", "2003", "Sleep")}
	mbid, qualifying := pickStrictReleaseGroup(cands, "Sleep", "Dopesmoker", intPtr(2003))
	assert.Equal(t, "", mbid)
	assert.Equal(t, 0, qualifying)
}

func dr(id int64, title string, year int, cover, source string) DiscogsRelease {
	return DiscogsRelease{ID: id, Title: title, Year: year, CoverImage: cover, SourceURL: source}
}

func TestPickStrictDiscogs_ContainmentMatch(t *testing.T) {
	cands := []DiscogsRelease{dr(111, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111")}
	got := pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003))
	if assert.NotNil(t, got) {
		assert.Equal(t, "https://i.discogs.com/a.jpg", got.imageURL)
		assert.Equal(t, "https://www.discogs.com/release/111", got.sourceURL)
	}
}

func TestPickStrictDiscogs_TitleNotContained(t *testing.T) {
	cands := []DiscogsRelease{dr(111, "Sleep - Holy Mountain", 1992, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111")}
	assert.Nil(t, pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003)))
}

func TestPickStrictDiscogs_ArtistNotContained(t *testing.T) {
	cands := []DiscogsRelease{dr(111, "Boris - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111")}
	assert.Nil(t, pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003)))
}

func TestPickStrictDiscogs_AmbiguousDistinctCovers(t *testing.T) {
	cands := []DiscogsRelease{
		dr(111, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111"),
		dr(222, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/b.jpg", "https://www.discogs.com/release/222"),
	}
	assert.Nil(t, pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003)), "two distinct covers → skip")
}

func TestPickStrictDiscogs_SameCoverNotAmbiguous(t *testing.T) {
	cands := []DiscogsRelease{
		dr(111, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111"),
		dr(222, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/222"),
	}
	got := pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003))
	if assert.NotNil(t, got) {
		assert.Equal(t, "https://i.discogs.com/a.jpg", got.imageURL)
	}
}

func TestPickStrictDiscogs_YearDisambiguates(t *testing.T) {
	cands := []DiscogsRelease{
		dr(111, "Sleep - Dopesmoker", 2003, "https://i.discogs.com/a.jpg", "https://www.discogs.com/release/111"),
		dr(222, "Sleep - Dopesmoker", 2004, "https://i.discogs.com/b.jpg", "https://www.discogs.com/release/222"),
	}
	got := pickStrictDiscogs(cands, "Sleep", "Dopesmoker", intPtr(2003))
	if assert.NotNil(t, got) {
		assert.Equal(t, "https://i.discogs.com/a.jpg", got.imageURL, "exact-year match wins")
	}
}

func TestDiscogsTitleContains_TokenBoundary(t *testing.T) {
	assert.True(t, discogsTitleContains("Sleep - Dopesmoker", "dopesmoker", "sleep"))
	assert.True(t, discogsTitleContains("Warpaint - Warpaint", "warpaint", "warpaint"))
	assert.False(t, discogsTitleContains("Warpaint - Warpaint", "war", "warpaint"),
		"'war' must not match inside 'warpaint'")
	assert.True(t, discogsTitleContains("Sleep - Holy Mountain", "holy mountain", "sleep"),
		"multi-word title phrase matches")
}

func TestValidCoverSourceURL(t *testing.T) {
	assert.True(t, validCoverSourceURL(coverArtSourceCAA, "https://musicbrainz.org/release-group/x"))
	assert.False(t, validCoverSourceURL(coverArtSourceCAA, "https://evil.test/release-group/x"))
	assert.True(t, validCoverSourceURL(coverArtSourceDiscogs, "https://www.discogs.com/release/1"))
	assert.True(t, validCoverSourceURL(coverArtSourceDiscogs, "https://discogs.com/release/1"))
	assert.False(t, validCoverSourceURL(coverArtSourceDiscogs, "http://www.discogs.com/release/1"), "non-https rejected")
	assert.False(t, validCoverSourceURL("spotify", "https://open.spotify.com/album/1"), "unknown source rejected")
}

func TestIsMusicBrainzWebURL(t *testing.T) {
	assert.True(t, isMusicBrainzWebURL("https://musicbrainz.org/release-group/x"))
	assert.False(t, isMusicBrainzWebURL("http://musicbrainz.org/release-group/x"))
	assert.False(t, isMusicBrainzWebURL("https://evil.musicbrainz.org.attacker.test/x"))
}

func TestIsDiscogsWebURL(t *testing.T) {
	assert.True(t, isDiscogsWebURL("https://www.discogs.com/release/1"))
	assert.True(t, isDiscogsWebURL("https://discogs.com/release/1"))
	assert.False(t, isDiscogsWebURL("https://i.discogs.com/release/1"), "image host is not the web host")
	assert.False(t, isDiscogsWebURL("https://discogs.com.attacker.test/release/1"))
}
