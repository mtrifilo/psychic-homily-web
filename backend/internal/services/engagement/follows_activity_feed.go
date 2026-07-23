package engagement

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

const (
	// followsActivityWindow bounds how far back announced shows/releases appear
	// in the personal Atom feed. RSS readers poll periodically; an unbounded
	// all-time dump would grow without limit for power followers.
	followsActivityWindow = 90 * 24 * time.Hour

	// followsActivityMaxItems caps merged show+release entries per response.
	followsActivityMaxItems = 100

	// atomFeedCacheTTL mirrors icsFeedCacheTTL: short private cache, cleared on
	// token create/delete so regenerate invalidates immediately.
	atomFeedCacheTTL = 2 * time.Minute
)

// activityKind distinguishes feed entry types in the merged stream.
type activityKind string

const (
	activityKindShow    activityKind = "show"
	activityKindRelease activityKind = "release"
)

type followsActivityItem struct {
	Kind      activityKind
	ID        uint
	Title     string
	URL       string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Atom XML types (RFC 4287). Hand-rolled so the payload validates without a
// third-party feed library.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
	Author  atomAuthor  `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string       `xml:"title"`
	ID        string       `xml:"id"`
	Updated   string       `xml:"updated"`
	Published string       `xml:"published"`
	Link      atomLink     `xml:"link"`
	Summary   string       `xml:"summary"`
	Category  atomCategory `xml:"category"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

// GenerateFollowsActivityFeed builds an Atom 1.0 feed of recent shows and
// releases involving artists the user follows (PSY-1505). Window: created_at
// within the last 90 days; merge-sorted by created_at DESC; capped at 100.
func (s *CalendarService) GenerateFollowsActivityFeed(userID uint, frontendURL string) ([]byte, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if cached, ok := s.atomFeedCache.Load(userID); ok {
		entry := cached.(icsFeedCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			out := make([]byte, len(entry.data))
			copy(out, entry.data)
			return out, nil
		}
		s.atomFeedCache.Delete(userID)
	}

	frontendURL = strings.TrimRight(frontendURL, "/")
	cutoff := time.Now().UTC().Add(-followsActivityWindow)

	shows, err := s.followedArtistShows(userID, cutoff)
	if err != nil {
		return nil, err
	}
	releases, err := s.followedArtistReleases(userID, cutoff)
	if err != nil {
		return nil, err
	}

	items := make([]followsActivityItem, 0, len(shows)+len(releases))
	for _, show := range shows {
		items = append(items, showToActivityItem(show, frontendURL))
	}
	for _, release := range releases {
		items = append(items, releaseToActivityItem(release, frontendURL))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			// Stable tie-break: shows before releases, then by ID.
			if items[i].Kind != items[j].Kind {
				return items[i].Kind == activityKindShow
			}
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > followsActivityMaxItems {
		items = items[:followsActivityMaxItems]
	}

	updated := time.Now().UTC()
	if len(items) > 0 {
		updated = items[0].UpdatedAt.UTC()
		if items[0].CreatedAt.After(updated) {
			updated = items[0].CreatedAt.UTC()
		}
	}

	feed := atomFeed{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   "Psychic Homily — Followed artists",
		ID:      fmt.Sprintf("tag:psychichomily.com,2026:follows-activity:%d", userID),
		Updated: updated.Format(time.RFC3339),
		Link: []atomLink{
			{Rel: "alternate", Href: frontendURL + "/library"},
		},
		Author: atomAuthor{Name: "Psychic Homily"},
	}

	feed.Entries = make([]atomEntry, 0, len(items))
	for _, item := range items {
		published := item.CreatedAt.UTC()
		entryUpdated := item.UpdatedAt.UTC()
		if entryUpdated.IsZero() {
			entryUpdated = published
		}
		feed.Entries = append(feed.Entries, atomEntry{
			Title:     item.Title,
			ID:        fmt.Sprintf("tag:psychichomily.com,2026:%s:%d", item.Kind, item.ID),
			Updated:   entryUpdated.Format(time.RFC3339),
			Published: published.Format(time.RFC3339),
			Link:      atomLink{Href: item.URL, Rel: "alternate"},
			Summary:   item.Summary,
			Category:  atomCategory{Term: string(item.Kind)},
		})
	}

	payload, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal atom feed: %w", err)
	}
	data := append([]byte(xml.Header), payload...)

	cachedCopy := make([]byte, len(data))
	copy(cachedCopy, data)
	s.atomFeedCache.Store(userID, icsFeedCacheEntry{
		data:      cachedCopy,
		expiresAt: time.Now().Add(atomFeedCacheTTL),
	})
	return data, nil
}

func (s *CalendarService) followedArtistShows(userID uint, cutoff time.Time) ([]catalogm.Show, error) {
	var shows []catalogm.Show
	err := s.db.
		Distinct("shows.*").
		Joins("JOIN show_artists sa ON sa.show_id = shows.id").
		Joins(
			"JOIN user_bookmarks ub ON ub.entity_id = sa.artist_id AND ub.user_id = ? AND ub.entity_type = ? AND ub.action = ?",
			userID, engagementm.BookmarkEntityArtist, engagementm.BookmarkActionFollow,
		).
		Where("shows.status = ? AND shows.is_cancelled = ? AND shows.created_at >= ?",
			catalogm.ShowStatusApproved, false, cutoff).
		Preload("Artists").
		Preload("Venues").
		Order("shows.created_at DESC").
		Limit(followsActivityMaxItems).
		Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch followed-artist shows: %w", err)
	}
	return shows, nil
}

func (s *CalendarService) followedArtistReleases(userID uint, cutoff time.Time) ([]catalogm.Release, error) {
	var releases []catalogm.Release
	err := s.db.
		Distinct("releases.*").
		Joins("JOIN artist_releases ar ON ar.release_id = releases.id").
		Joins(
			"JOIN user_bookmarks ub ON ub.entity_id = ar.artist_id AND ub.user_id = ? AND ub.entity_type = ? AND ub.action = ?",
			userID, engagementm.BookmarkEntityArtist, engagementm.BookmarkActionFollow,
		).
		Where("releases.created_at >= ?", cutoff).
		Preload("Artists").
		Order("releases.created_at DESC").
		Limit(followsActivityMaxItems).
		Find(&releases).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch followed-artist releases: %w", err)
	}
	return releases, nil
}

func showToActivityItem(show catalogm.Show, frontendURL string) followsActivityItem {
	slug := entitySlug(show.Slug, show.ID)
	url := fmt.Sprintf("%s/shows/%s", frontendURL, slug)

	artistNames := make([]string, 0, len(show.Artists))
	for _, a := range show.Artists {
		artistNames = append(artistNames, a.Name)
	}

	var summaryParts []string
	if len(artistNames) > 0 {
		summaryParts = append(summaryParts, "Artists: "+strings.Join(artistNames, ", "))
	}
	if len(show.Venues) > 0 {
		v := show.Venues[0]
		loc := v.Name
		if v.City != "" {
			loc += ", " + v.City
		}
		if v.State != "" {
			loc += ", " + v.State
		}
		summaryParts = append(summaryParts, "Venue: "+loc)
	}
	summaryParts = append(summaryParts, "Date: "+show.EventDate.UTC().Format("2006-01-02"))
	summaryParts = append(summaryParts, url)

	title := show.Title
	if title == "" && len(artistNames) > 0 {
		title = strings.Join(artistNames, ", ")
	}
	if title == "" {
		title = fmt.Sprintf("Show #%d", show.ID)
	}

	return followsActivityItem{
		Kind:      activityKindShow,
		ID:        show.ID,
		Title:     "Show: " + title,
		URL:       url,
		Summary:   strings.Join(summaryParts, "\n"),
		CreatedAt: show.CreatedAt,
		UpdatedAt: show.UpdatedAt,
	}
}

func releaseToActivityItem(release catalogm.Release, frontendURL string) followsActivityItem {
	slug := entitySlug(release.Slug, release.ID)
	url := fmt.Sprintf("%s/releases/%s", frontendURL, slug)

	artistNames := make([]string, 0, len(release.Artists))
	for _, a := range release.Artists {
		artistNames = append(artistNames, a.Name)
	}

	var summaryParts []string
	if len(artistNames) > 0 {
		summaryParts = append(summaryParts, "Artists: "+strings.Join(artistNames, ", "))
	}
	if release.ReleaseType != "" {
		summaryParts = append(summaryParts, "Type: "+string(release.ReleaseType))
	}
	if release.ReleaseDate != nil && *release.ReleaseDate != "" {
		summaryParts = append(summaryParts, "Released: "+*release.ReleaseDate)
	} else if release.ReleaseYear != nil {
		summaryParts = append(summaryParts, fmt.Sprintf("Year: %d", *release.ReleaseYear))
	}
	summaryParts = append(summaryParts, url)

	title := release.Title
	if title == "" {
		title = fmt.Sprintf("Release #%d", release.ID)
	}
	if len(artistNames) > 0 {
		title = artistNames[0] + " — " + title
	}

	return followsActivityItem{
		Kind:      activityKindRelease,
		ID:        release.ID,
		Title:     "Release: " + title,
		URL:       url,
		Summary:   strings.Join(summaryParts, "\n"),
		CreatedAt: release.CreatedAt,
		UpdatedAt: release.UpdatedAt,
	}
}

func entitySlug(slug *string, id uint) string {
	if slug != nil && strings.TrimSpace(*slug) != "" {
		return strings.TrimSpace(*slug)
	}
	return fmt.Sprintf("%d", id)
}
