package engagement

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

func TestEntitySlug(t *testing.T) {
	s := "my-slug"
	assert.Equal(t, "my-slug", entitySlug(&s, 9))
	empty := "  "
	assert.Equal(t, "9", entitySlug(&empty, 9))
	assert.Equal(t, "9", entitySlug(nil, 9))
}

func TestShowToActivityItem(t *testing.T) {
	slug := "phoenix-show"
	show := catalogm.Show{
		ID:        42,
		Title:     "Night Show",
		Slug:      &slug,
		EventDate: time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Artists:   []catalogm.Artist{{ID: 1, Name: "Band A"}},
		Venues:    []catalogm.Venue{{ID: 1, Name: "The Bar", City: "Phoenix", State: "AZ"}},
	}
	item := showToActivityItem(show, "https://psychichomily.com")
	assert.Equal(t, activityKindShow, item.Kind)
	assert.Equal(t, uint(42), item.ID)
	assert.Equal(t, "Show: Night Show", item.Title)
	assert.Equal(t, "https://psychichomily.com/shows/phoenix-show", item.URL)
	assert.Contains(t, item.Summary, "Band A")
	assert.Contains(t, item.Summary, "The Bar, Phoenix, AZ")
}

func TestReleaseToActivityItem(t *testing.T) {
	slug := "new-lp"
	year := 2026
	date := "2026-07-01"
	release := catalogm.Release{
		ID:          7,
		Title:       "New LP",
		Slug:        &slug,
		ReleaseType: catalogm.ReleaseTypeLP,
		ReleaseYear: &year,
		ReleaseDate: &date,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Artists:     []catalogm.Artist{{ID: 1, Name: "Band A"}},
	}
	item := releaseToActivityItem(release, "https://psychichomily.com")
	assert.Equal(t, activityKindRelease, item.Kind)
	assert.Equal(t, "Release: Band A — New LP", item.Title)
	assert.Equal(t, "https://psychichomily.com/releases/new-lp", item.URL)
	assert.Contains(t, item.Summary, "Type: lp")
	assert.Contains(t, item.Summary, "Released: 2026-07-01")
}

func TestFollowsActivityFeedURL(t *testing.T) {
	url := followsActivityFeedURL("http://localhost:8080/", "phcal_abc")
	assert.Equal(t, "http://localhost:8080/feeds/phcal_abc/follows.atom", url)
}

func TestAtomFeedMarshal_Structural(t *testing.T) {
	// Structural check equivalent to feedvalidator: required Atom elements,
	// xmlns, and well-formed XML (PSY-1505 acceptance).
	feed := atomFeed{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   "Psychic Homily — Followed artists",
		ID:      "tag:psychichomily.com,2026:follows-activity:1",
		Updated: time.Now().UTC().Format(time.RFC3339),
		Link:    []atomLink{{Rel: "alternate", Href: "https://psychichomily.com/library"}},
		Author:  atomAuthor{Name: "Psychic Homily"},
		Entries: []atomEntry{{
			Title:     "Show: Test",
			ID:        "tag:psychichomily.com,2026:show:1",
			Updated:   time.Now().UTC().Format(time.RFC3339),
			Published: time.Now().UTC().Format(time.RFC3339),
			Link:      atomLink{Href: "https://psychichomily.com/shows/1", Rel: "alternate"},
			Summary:   "Artists: Band A",
			Category:  atomCategory{Term: "show"},
		}},
	}
	payload, err := xml.MarshalIndent(feed, "", "  ")
	require.NoError(t, err)
	body := xml.Header + string(payload)

	assert.True(t, strings.HasPrefix(body, xml.Header))
	assert.Contains(t, body, `xmlns="http://www.w3.org/2005/Atom"`)
	assert.Contains(t, body, "<feed")
	assert.Contains(t, body, "<title>")
	assert.Contains(t, body, "<id>")
	assert.Contains(t, body, "<updated>")
	assert.Contains(t, body, "<entry>")
	assert.Contains(t, body, "<category term=\"show\"")

	var parsed atomFeed
	require.NoError(t, xml.Unmarshal(payload, &parsed))
	assert.Equal(t, "Psychic Homily — Followed artists", parsed.Title)
	require.Len(t, parsed.Entries, 1)
	assert.Equal(t, "Show: Test", parsed.Entries[0].Title)
}

func (suite *CalendarIntegrationTestSuite) TestGenerateFollowsActivityFeed_ShowAndRelease() {
	user := suite.createTestUser(true)

	artist := catalogm.Artist{Name: "Followed Band"}
	suite.Require().NoError(suite.db.Create(&artist).Error)

	otherArtist := catalogm.Artist{Name: "Unfollowed Band"}
	suite.Require().NoError(suite.db.Create(&otherArtist).Error)

	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID:     user.ID,
		EntityType: engagementm.BookmarkEntityArtist,
		EntityID:   artist.ID,
		Action:     engagementm.BookmarkActionFollow,
		CreatedAt:  time.Now(),
	}).Error)

	showSlug := "followed-band-show"
	show := catalogm.Show{
		Title:     "Followed Band Live",
		Slug:      &showSlug,
		EventDate: time.Now().Add(48 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	suite.Require().NoError(suite.db.Create(&show).Error)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)`,
		show.ID, artist.ID,
	).Error)

	// Unrelated show (unfollowed artist) must not appear.
	otherShow := catalogm.Show{
		Title:     "Other Show",
		EventDate: time.Now().Add(72 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	suite.Require().NoError(suite.db.Create(&otherShow).Error)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)`,
		otherShow.ID, otherArtist.ID,
	).Error)

	releaseSlug := "followed-band-lp"
	release := catalogm.Release{
		Title:       "New Album",
		Slug:        &releaseSlug,
		ReleaseType: catalogm.ReleaseTypeLP,
		CreatedAt:   time.Now().Add(-30 * time.Minute),
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
	}
	suite.Require().NoError(suite.db.Create(&release).Error)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO artist_releases (artist_id, release_id, role, position) VALUES (?, ?, 'main', 0)`,
		artist.ID, release.ID,
	).Error)

	data, err := suite.svc.GenerateFollowsActivityFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)
	body := string(data)

	suite.True(strings.HasPrefix(body, xml.Header))
	suite.Contains(body, `xmlns="http://www.w3.org/2005/Atom"`)
	suite.Contains(body, "Show: Followed Band Live")
	suite.Contains(body, "https://psychichomily.com/shows/followed-band-show")
	suite.Contains(body, "Release: Followed Band — New Album")
	suite.Contains(body, "https://psychichomily.com/releases/followed-band-lp")
	suite.NotContains(body, "Other Show")

	var parsed atomFeed
	suite.Require().NoError(xml.Unmarshal([]byte(strings.TrimPrefix(body, xml.Header)), &parsed))
	suite.GreaterOrEqual(len(parsed.Entries), 2)
}

func (suite *CalendarIntegrationTestSuite) TestGenerateFollowsActivityFeed_RegenerateInvalidatesCache() {
	user := suite.createTestUser(true)
	artist := catalogm.Artist{Name: "Cache Band"}
	suite.Require().NoError(suite.db.Create(&artist).Error)
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID:     user.ID,
		EntityType: engagementm.BookmarkEntityArtist,
		EntityID:   artist.ID,
		Action:     engagementm.BookmarkActionFollow,
		CreatedAt:  time.Now(),
	}).Error)

	first, err := suite.svc.GenerateFollowsActivityFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)

	show := catalogm.Show{
		Title:     "After Cache Show",
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	suite.Require().NoError(suite.db.Create(&show).Error)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)`,
		show.ID, artist.ID,
	).Error)

	// Without invalidate, short cache would still return the empty feed.
	cached, err := suite.svc.GenerateFollowsActivityFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)
	suite.Equal(string(first), string(cached))
	suite.NotContains(string(cached), "After Cache Show")

	_, err = suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	fresh, err := suite.svc.GenerateFollowsActivityFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)
	suite.Contains(string(fresh), "After Cache Show")
}
