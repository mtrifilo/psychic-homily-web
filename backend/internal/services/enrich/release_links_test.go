package enrich

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
)

type fakeReleaseLinkStore struct {
	candidates []releaseLinkCandidate
	// nowLinked simulates links appearing AFTER the candidate snapshot (the
	// TOCTOU window): "releaseID/platform" entries make the pre-write re-check
	// report the link as present.
	nowLinked map[string]bool
	recheck   []string // every (releaseID/platform) the core re-checked before writing
}

func (f *fakeReleaseLinkStore) ReleaseLinkCandidates(limit int) ([]releaseLinkCandidate, error) {
	if limit > 0 && limit < len(f.candidates) {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func (f *fakeReleaseLinkStore) ReleaseHasPlatformLink(releaseID uint, platform string) (bool, error) {
	key := fmt.Sprintf("%d/%s", releaseID, platform)
	f.recheck = append(f.recheck, key)
	return f.nowLinked[key], nil
}

type fakeReleaseBrowser struct {
	byRG  map[string][]pipeline.MBReleaseResult
	errRG map[string]error
	calls []string
}

func (f *fakeReleaseBrowser) BrowseReleaseURLRelations(_ context.Context, rgMBID string) ([]pipeline.MBReleaseResult, error) {
	f.calls = append(f.calls, rgMBID)
	if err, ok := f.errRG[rgMBID]; ok {
		return nil, err
	}
	return f.byRG[rgMBID], nil
}

type fakeLinkWriter struct {
	writes []ReleaseLinkFill
	failOn string // platform to fail on ("" = never)
}

func (f *fakeLinkWriter) AddExternalLink(releaseID uint, platform, url string) (*contracts.ReleaseExternalLinkResponse, error) {
	if platform == f.failOn {
		return nil, fmt.Errorf("boom")
	}
	f.writes = append(f.writes, ReleaseLinkFill{ReleaseID: releaseID, Platform: platform, URL: url})
	return &contracts.ReleaseExternalLinkResponse{Platform: platform, URL: url}, nil
}

func mbRel(url string, ended bool) pipeline.MBURLRelation {
	r := pipeline.MBURLRelation{Ended: ended}
	r.URL.Resource = url
	return r
}

func candidate(id uint, title, rg string, hasBC, hasSP bool) releaseLinkCandidate {
	return releaseLinkCandidate{
		release: catalogm.Release{
			ID:                        id,
			Title:                     title,
			MusicBrainzReleaseGroupID: &rg,
		},
		hasBandcamp: hasBC,
		hasSpotify:  hasSP,
	}
}

const rgA = "11111111-1111-1111-1111-111111111111"
const rgB = "22222222-2222-2222-2222-222222222222"

func TestBackfillReleaseLinks_FillsBothPlatformsAndSharesRGBrowse(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "Punisher", rgA, false, false),
		candidate(2, "Punisher (reissue)", rgA, false, false), // same RG — no second browse
	}}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{
			Status: "Official",
			Relations: []pipeline.MBURLRelation{
				mbRel("https://phoebe.bandcamp.com/album/punisher", false),
				mbRel("https://open.spotify.com/album/6Pp6qGEywDdofgFC1oFbSH", false),
			},
		}},
	}}
	writer := &fakeLinkWriter{}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, report.ReleasesScanned)
	assert.Equal(t, 1, report.RGsBrowsed, "one browse serves both releases in the RG")
	assert.Equal(t, []string{rgA}, browser.calls)
	assert.Equal(t, 2, report.FilledBandcamp)
	assert.Equal(t, 2, report.FilledSpotify)
	assert.Len(t, writer.writes, 4)
	assert.Empty(t, report.Errors)
}

func TestBackfillReleaseLinks_FillWhenEmptyPerPlatform(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "Has BC already", rgA, true, false), // only spotify missing
	}}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{
			Status: "Official",
			Relations: []pipeline.MBURLRelation{
				mbRel("https://phoebe.bandcamp.com/album/punisher", false),
				mbRel("https://open.spotify.com/album/abc", false),
			},
		}},
	}}
	writer := &fakeLinkWriter{}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, 0, report.FilledBandcamp, "existing bandcamp link untouched")
	assert.Equal(t, 1, report.FilledSpotify)
	require.Len(t, writer.writes, 1)
	assert.Equal(t, contracts.MusicPlatformSpotify, writer.writes[0].Platform)
}

func TestBackfillReleaseLinks_DryRunWritesNothing(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "Punisher", rgA, false, false),
	}}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{Status: "Official", Relations: []pipeline.MBURLRelation{
			mbRel("https://phoebe.bandcamp.com/album/punisher", false),
		}}},
	}}

	// nil writer is allowed in dry-run and MUST never be called.
	report, err := backfillReleaseLinks(store, browser, nil, ReleaseLinksOptions{DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, 1, report.FilledBandcamp)
	require.Len(t, report.Fills, 1)
	assert.Equal(t, "https://phoebe.bandcamp.com/album/punisher", report.Fills[0].URL)
}

func TestBackfillReleaseLinks_LiveRequiresWriter(t *testing.T) {
	_, err := backfillReleaseLinks(&fakeReleaseLinkStore{}, &fakeReleaseBrowser{}, nil, ReleaseLinksOptions{})
	require.Error(t, err)
}

func TestBackfillReleaseLinks_BrowseErrorSkipsSiblingsAndCounts(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "A", rgA, false, false),
		candidate(2, "A sibling", rgA, false, false), // same failed RG — no re-browse
		candidate(3, "B", rgB, false, false),
	}}
	browser := &fakeReleaseBrowser{
		errRG: map[string]error{rgA: fmt.Errorf("mb down")},
		byRG: map[string][]pipeline.MBReleaseResult{
			rgB: {{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://b.bandcamp.com/album/b", false),
			}}},
		},
	}
	writer := &fakeLinkWriter{}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{rgA, rgB}, browser.calls, "failed RG browsed once, not per sibling")
	assert.Len(t, report.Errors, 1)
	assert.Equal(t, 1, report.FilledBandcamp, "healthy RG still processed")
}

func TestBackfillReleaseLinks_WriteErrorReported(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "Punisher", rgA, false, false),
	}}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{Status: "Official", Relations: []pipeline.MBURLRelation{
			mbRel("https://phoebe.bandcamp.com/album/punisher", false),
			mbRel("https://open.spotify.com/album/abc", false),
		}}},
	}}
	writer := &fakeLinkWriter{failOn: contracts.MusicPlatformBandcamp}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, 0, report.FilledBandcamp, "failed write not counted")
	assert.Equal(t, 1, report.FilledSpotify, "other platform still written")
	assert.Len(t, report.Errors, 1)
}

func TestPickReleaseURL_PreferenceOrder(t *testing.T) {
	t.Run("album beats track within allowed statuses", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/track/official-track", false),
				mbRel("https://x.bandcamp.com/album/official-album", false),
			}},
		}
		u, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		require.True(t, ok)
		assert.Equal(t, "https://x.bandcamp.com/album/official-album", u)
	})

	t.Run("album is primary: a status-less album beats an Official track", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/track/official-track", false),
			}},
			{Status: "", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/statusless-album", false),
			}},
		}
		u, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		require.True(t, ok)
		assert.Equal(t, "https://x.bandcamp.com/album/statusless-album", u)
	})

	t.Run("status floor: bootleg/promo-only RG yields nothing", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Bootleg", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/bootleg-album", false),
			}},
			{Status: "Promotion", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/promo-album", false),
			}},
		}
		_, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		assert.False(t, ok, "non-Official statuses are a floor, not a preference")
	})

	t.Run("ended relations skipped", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/delisted", true),
			}},
		}
		_, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		assert.False(t, ok)
	})

	t.Run("wrong platform not returned", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://open.spotify.com/album/abc", false),
			}},
		}
		_, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		assert.False(t, ok)
	})

	t.Run("first result wins full ties", func(t *testing.T) {
		rels := []pipeline.MBReleaseResult{
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/first", false),
			}},
			{Status: "Official", Relations: []pipeline.MBURLRelation{
				mbRel("https://x.bandcamp.com/album/second", false),
			}},
		}
		u, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp)
		require.True(t, ok)
		assert.Equal(t, "https://x.bandcamp.com/album/first", u)
	})
}

func TestBackfillReleaseLinks_PreWriteRecheckSkipsRacedLink(t *testing.T) {
	store := &fakeReleaseLinkStore{
		candidates: []releaseLinkCandidate{candidate(1, "Punisher", rgA, false, false)},
		// bandcamp link appeared AFTER the snapshot (admin add / concurrent run)
		nowLinked: map[string]bool{"1/bandcamp": true},
	}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{Status: "Official", Relations: []pipeline.MBURLRelation{
			mbRel("https://phoebe.bandcamp.com/album/punisher", false),
			mbRel("https://open.spotify.com/album/abc", false),
		}}},
	}}
	writer := &fakeLinkWriter{}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, 0, report.FilledBandcamp, "raced link not double-written")
	assert.Equal(t, 1, report.FilledSpotify)
	require.Len(t, writer.writes, 1, "only the still-missing platform written")
	assert.Contains(t, store.recheck, "1/bandcamp", "live path re-checked before write")
	assert.Empty(t, report.Errors, "a raced fill is not an error")
}

func TestBackfillReleaseLinks_DryRunSkipsRecheck(t *testing.T) {
	store := &fakeReleaseLinkStore{
		candidates: []releaseLinkCandidate{candidate(1, "Punisher", rgA, false, false)},
	}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{Status: "Official", Relations: []pipeline.MBURLRelation{
			mbRel("https://phoebe.bandcamp.com/album/punisher", false),
		}}},
	}}

	report, err := backfillReleaseLinks(store, browser, nil, ReleaseLinksOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 1, report.FilledBandcamp)
	assert.Empty(t, store.recheck, "dry-run does no pre-write re-checks")
}

func TestBackfillReleaseLinks_InvalidStoredMBIDNeverBrowsedAndSurfaced(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "Corrupted", "not-a-uuid", false, false),
		candidate(2, "Healthy", rgA, false, false),
	}}
	browser := &fakeReleaseBrowser{byRG: map[string][]pipeline.MBReleaseResult{
		rgA: {{Status: "Official", Relations: []pipeline.MBURLRelation{
			mbRel("https://b.bandcamp.com/album/b", false),
		}}},
	}}
	writer := &fakeLinkWriter{}

	report, err := backfillReleaseLinks(store, browser, writer, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{rgA}, browser.calls, "malformed RG-MBID never browsed")
	require.Len(t, report.Errors, 1, "corrupted key surfaced, not silently skipped")
	assert.Contains(t, report.Errors[0], "invalid stored RG-MBID")
	assert.Equal(t, 1, report.FilledBandcamp, "healthy sibling unaffected")
}

func TestBackfillReleaseLinks_FailedRGSiblingsCounted(t *testing.T) {
	store := &fakeReleaseLinkStore{candidates: []releaseLinkCandidate{
		candidate(1, "A", rgA, false, false),
		candidate(2, "A sibling", rgA, false, false),
		candidate(3, "A sibling 2", rgA, false, false),
	}}
	browser := &fakeReleaseBrowser{errRG: map[string]error{rgA: fmt.Errorf("mb down")}}

	report, err := backfillReleaseLinks(store, browser, &fakeLinkWriter{}, ReleaseLinksOptions{})
	require.NoError(t, err)

	assert.Len(t, report.Errors, 1, "one error for the failed browse")
	assert.Equal(t, 2, report.ReleasesSkippedFailedRG, "siblings appear in the report arithmetic")
	assert.Equal(t, 0, report.ReleasesNoLinks)
}
