package admin

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// DataQualityService handles data quality analysis for the admin dashboard.
type DataQualityService struct {
	db *gorm.DB
}

// NewDataQualityService creates a new data quality service.
func NewDataQualityService(database *gorm.DB) *DataQualityService {
	if database == nil {
		database = db.GetDB()
	}
	return &DataQualityService{db: database}
}

// categoryDefinitions maps category keys to their metadata.
var categoryDefinitions = map[string]struct {
	Label       string
	EntityType  string
	Description string
}{
	"artists_missing_links": {
		Label:       "Artists Missing Links",
		EntityType:  "artist",
		Description: "Artists with no social links or website set",
	},
	"artists_missing_location": {
		Label:       "Artists Missing Location",
		EntityType:  "artist",
		Description: "Artists with no city or state set",
	},
	"artists_no_aliases": {
		Label:       "Artists Without Aliases",
		EntityType:  "artist",
		Description: "Artists with 5+ shows but no aliases (potential disambiguation needed)",
	},
	"venues_missing_social": {
		Label:       "Venues Missing Social",
		EntityType:  "venue",
		Description: "Venues with no social links or website set",
	},
	"venues_unverified_with_shows": {
		Label:       "Unverified Venues With Shows",
		EntityType:  "venue",
		Description: "Unverified venues that have 3+ approved shows",
	},
	"shows_no_billing_order": {
		Label:       "Shows Without Billing Order",
		EntityType:  "show",
		Description: "Upcoming shows with 2+ artists but no billing differentiation",
	},
	"shows_missing_price": {
		Label:       "Shows Missing Price",
		EntityType:  "show",
		Description: "Upcoming approved shows with no price set",
	},
	"releases_missing_year": {
		Label:       "Releases Missing Year",
		EntityType:  "release",
		Description: "Releases with no release year set",
	},
}

// categoryOrder defines the display order for categories.
var categoryOrder = []string{
	"artists_missing_links",
	"artists_missing_location",
	"artists_no_aliases",
	"venues_missing_social",
	"venues_unverified_with_shows",
	"shows_no_billing_order",
	"shows_missing_price",
	"releases_missing_year",
}

// --- Loose Ends contribution categories (PSY-1483) ---
//
// These two categories are viewer-aware, high-signal gap lists surfaced only
// on /contribute (never the admin dashboard, which keeps categoryOrder). Per
// the PSY-1426 spike, "missing links" here is the NARROW definition — no
// Bandcamp AND no Spotify — distinct from the catalog-wide
// artists_missing_links category, which requires every social/website field
// to be empty. The frontend labels the band "Loose Ends" (PSY-1484); the
// backend just exposes the category keys/labels below.
const (
	categoryFollowedArtistsMissingLinks = "followed_artists_missing_links"
	categoryChartingArtistsMissingLinks = "charting_artists_missing_links"

	// looseEndsMaxItems caps each Loose Ends list per response (PSY-1426
	// decision): the entity-edit UX degrades past a couple dozen rows, and a
	// hard cap keeps the rotated slice small and cache-friendly.
	looseEndsMaxItems = 25

	// chartingWindowDays mirrors the charts default window (quarter = rolling
	// 90 days; see catalog.chartWindowBounds and ChartWindow.OrDefault). A
	// "charting" artist appears on >= chartingMinAppearances approved,
	// non-cancelled shows inside this window. Keep in sync with the charts
	// quarter default.
	chartingWindowDays = 90

	// chartingMinAppearances is the in-window show-appearance threshold for an
	// artist to count as "currently charting" (PSY-1426).
	chartingMinAppearances = 2
)

// looseEndsMissingLinksSQL is the narrow "missing links" predicate for the
// Loose Ends categories: no Bandcamp AND no Spotify (PSY-1426). Applies to the
// artists table aliased `a`.
const looseEndsMissingLinksSQL = `a.bandcamp IS NULL AND a.spotify IS NULL`

// looseEndsRotationSQL produces the stable daily rotation ordering key
// (PSY-1426): md5 over the artist id, a viewer key, and the UTC date. This
// replaces raw RANDOM() (which is cache-hostile and reshuffles every request)
// with a slice that is stable within a UTC day for a given viewer. The two
// placeholders bind the viewer key and the UTC date (computed in Go so the
// day boundary can't drift with the DB session timezone).
const looseEndsRotationSQL = `md5(a.id::text || ':' || ? || ':' || ?)`

// looseEndsDefinitions holds the display metadata for the Loose Ends
// categories, mirroring categoryDefinitions but kept separate so the admin
// dashboard's category set (categoryOrder/categoryDefinitions) stays untouched.
var looseEndsDefinitions = map[string]struct {
	Label       string
	EntityType  string
	Description string
}{
	categoryFollowedArtistsMissingLinks: {
		Label:       "Followed Artists Missing Links",
		EntityType:  "artist",
		Description: "Artists you follow that have no Bandcamp or Spotify link",
	},
	categoryChartingArtistsMissingLinks: {
		Label:       "Charting Artists Missing Links",
		EntityType:  "artist",
		Description: "Artists active on recent shows that have no Bandcamp or Spotify link",
	},
}

// looseEndsCategoryOrder defines the display order for the Loose Ends
// categories on /contribute.
var looseEndsCategoryOrder = []string{
	categoryFollowedArtistsMissingLinks,
	categoryChartingArtistsMissingLinks,
}

// GetSummary returns counts per data quality category.
func (s *DataQualityService) GetSummary() (*contracts.DataQualitySummary, error) {
	summary := &contracts.DataQualitySummary{}
	totalItems := 0

	for _, key := range categoryOrder {
		def := categoryDefinitions[key]
		count, err := s.getCategoryCount(key)
		if err != nil {
			return nil, fmt.Errorf("counting category %s: %w", key, err)
		}
		summary.Categories = append(summary.Categories, contracts.DataQualityCategory{
			Key:         key,
			Label:       def.Label,
			EntityType:  def.EntityType,
			Count:       count,
			Description: def.Description,
		})
		totalItems += count
	}

	summary.TotalItems = totalItems
	return summary, nil
}

// GetCategoryItems returns paginated items for a specific data quality category.
func (s *DataQualityService) GetCategoryItems(category string, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	if _, ok := categoryDefinitions[category]; !ok {
		return nil, 0, apperrors.ErrDataQualityUnknownCategory(category)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	switch category {
	case "artists_missing_links":
		return s.getArtistsMissingLinks(limit, offset)
	case "artists_missing_location":
		return s.getArtistsMissingLocation(limit, offset)
	case "artists_no_aliases":
		return s.getArtistsNoAliases(limit, offset)
	case "venues_missing_social":
		return s.getVenuesMissingSocial(limit, offset)
	case "venues_unverified_with_shows":
		return s.getVenuesUnverifiedWithShows(limit, offset)
	case "shows_no_billing_order":
		return s.getShowsNoBillingOrder(limit, offset)
	case "shows_missing_price":
		return s.getShowsMissingPrice(limit, offset)
	case "releases_missing_year":
		return s.getReleasesMissingYear(limit, offset)
	default:
		return nil, 0, apperrors.ErrDataQualityUnknownCategory(category)
	}
}

// --- Contribute surface (Loose Ends categories, PSY-1483) ---

// GetContributeSummary returns the standard data-quality categories plus the
// viewer-aware Loose Ends categories. The followed list requires an
// authenticated viewer and is omitted for anonymous callers; the charting
// list is public. Category counts are the true totals; the item lists are
// capped and rotated (see GetContributeCategoryItems).
func (s *DataQualityService) GetContributeSummary(viewerID *uint) (*contracts.DataQualitySummary, error) {
	summary, err := s.GetSummary()
	if err != nil {
		return nil, err
	}

	for _, key := range looseEndsCategoryOrder {
		if key == categoryFollowedArtistsMissingLinks && viewerID == nil {
			// The followed list is personal; there is nothing to show an
			// anonymous caller, so omit the category entirely rather than
			// surface a permanent zero.
			continue
		}
		count, err := s.getLooseEndsCount(key, viewerID)
		if err != nil {
			return nil, fmt.Errorf("counting category %s: %w", key, err)
		}
		def := looseEndsDefinitions[key]
		summary.Categories = append(summary.Categories, contracts.DataQualityCategory{
			Key:         key,
			Label:       def.Label,
			EntityType:  def.EntityType,
			Count:       count,
			Description: def.Description,
		})
		summary.TotalItems += count
	}

	return summary, nil
}

// GetContributeCategoryItems returns paginated items for a contribution
// category. Global categories delegate to GetCategoryItems; the Loose Ends
// categories are capped at looseEndsMaxItems and rotated stably per viewer per
// UTC day. An anonymous request for the followed list returns an empty list
// (the category is authed-only) rather than an error, matching the summary's
// omit-for-anon behaviour.
func (s *DataQualityService) GetContributeCategoryItems(category string, viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	switch category {
	case categoryFollowedArtistsMissingLinks:
		if viewerID == nil {
			return []*contracts.DataQualityItem{}, 0, nil
		}
		return s.getFollowedArtistsMissingLinks(*viewerID, looseEndsLimit(limit), offset)
	case categoryChartingArtistsMissingLinks:
		return s.getChartingArtistsMissingLinks(viewerID, looseEndsLimit(limit), offset)
	default:
		return s.GetCategoryItems(category, limit, offset)
	}
}

// looseEndsLimit clamps a requested page size to [1, looseEndsMaxItems],
// defaulting to the cap when unset.
func looseEndsLimit(limit int) int {
	if limit <= 0 || limit > looseEndsMaxItems {
		return looseEndsMaxItems
	}
	return limit
}

// viewerRotationKey is the per-viewer rotation seed: the user id when authed,
// or a fixed "global" slice for anonymous callers (keeps the anon charting
// list cache-friendly, per PSY-1426).
func viewerRotationKey(viewerID *uint) string {
	if viewerID == nil {
		return "global"
	}
	return strconv.FormatUint(uint64(*viewerID), 10)
}

// rotationDay is the UTC calendar day used in the rotation key. Computed in Go
// so the day boundary is pinned to UTC regardless of the DB session timezone.
func rotationDay() string {
	return time.Now().UTC().Format("2006-01-02")
}

// chartingWindowBounds returns the [start, end] event-date bounds for the
// charting window: the rolling chartingWindowDays ending now, with the lower
// bound truncated to midnight UTC (event dates are midnight timestamps, so a
// time-of-day lower bound would exclude a show exactly N days ago) — the same
// convention as catalog.chartWindowBounds.
func chartingWindowBounds() (start, end time.Time) {
	end = time.Now().UTC()
	start = end.AddDate(0, 0, -chartingWindowDays).Truncate(24 * time.Hour)
	return start, end
}

func (s *DataQualityService) getLooseEndsCount(category string, viewerID *uint) (int, error) {
	switch category {
	case categoryFollowedArtistsMissingLinks:
		if viewerID == nil {
			return 0, nil
		}
		return s.countFollowedArtistsMissingLinks(*viewerID)
	case categoryChartingArtistsMissingLinks:
		return s.countChartingArtistsMissingLinks()
	default:
		return 0, apperrors.ErrDataQualityUnknownCategory(category)
	}
}

func (s *DataQualityService) countFollowedArtistsMissingLinks(viewerID uint) (int, error) {
	var count int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM artists a
		WHERE `+looseEndsMissingLinksSQL+`
		  AND EXISTS (
		    SELECT 1 FROM user_bookmarks ub
		    WHERE ub.entity_id = a.id
		      AND ub.entity_type = 'artist'
		      AND ub.action = 'follow'
		      AND ub.user_id = ?
		  )
	`, viewerID).Scan(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *DataQualityService) countChartingArtistsMissingLinks() (int, error) {
	windowStart, windowEnd := chartingWindowBounds()
	var count int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT a.id
			FROM artists a
			JOIN show_artists sa ON sa.artist_id = a.id
			JOIN shows s ON s.id = sa.show_id
			  AND s.status = 'approved'
			  AND s.is_cancelled = FALSE
			  AND s.event_date >= ? AND s.event_date <= ?
			WHERE `+looseEndsMissingLinksSQL+`
			GROUP BY a.id
			HAVING COUNT(DISTINCT sa.show_id) >= ?
		) charting
	`, windowStart, windowEnd, chartingMinAppearances).Scan(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *DataQualityService) getFollowedArtistsMissingLinks(viewerID uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	total, err := s.countFollowedArtistsMissingLinks(viewerID)
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	// show_count mirrors getArtistsMissingLinks (all linked show_artists rows)
	// so the item shape is consistent across artist gap categories. The md5
	// rotation is tiebroken by a.id so pagination is duplicate-free within a
	// day even in the (astronomically unlikely) event of a digest collision.
	err = s.db.Raw(`
		SELECT a.id, a.name, a.slug, COUNT(sa.show_id) as show_count
		FROM artists a
		LEFT JOIN show_artists sa ON sa.artist_id = a.id
		LEFT JOIN shows s ON s.id = sa.show_id AND s.status = 'approved'
		WHERE `+looseEndsMissingLinksSQL+`
		  AND EXISTS (
		    SELECT 1 FROM user_bookmarks ub
		    WHERE ub.entity_id = a.id
		      AND ub.entity_type = 'artist'
		      AND ub.action = 'follow'
		      AND ub.user_id = ?
		  )
		GROUP BY a.id
		ORDER BY `+looseEndsRotationSQL+`, a.id
		LIMIT ? OFFSET ?
	`, viewerID, viewerRotationKey(&viewerID), rotationDay(), limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     "Followed artist with no Bandcamp or Spotify link",
			ShowCount:  r.ShowCount,
		})
	}
	return items, int64(total), nil
}

func (s *DataQualityService) getChartingArtistsMissingLinks(viewerID *uint, limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	total, err := s.countChartingArtistsMissingLinks()
	if err != nil {
		return nil, 0, err
	}

	windowStart, windowEnd := chartingWindowBounds()
	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	// show_count is in-window appearances (the same set the >= threshold
	// counts), so it explains WHY the artist qualifies.
	err = s.db.Raw(`
		SELECT a.id, a.name, a.slug, COUNT(DISTINCT sa.show_id) as show_count
		FROM artists a
		JOIN show_artists sa ON sa.artist_id = a.id
		JOIN shows s ON s.id = sa.show_id
		  AND s.status = 'approved'
		  AND s.is_cancelled = FALSE
		  AND s.event_date >= ? AND s.event_date <= ?
		WHERE `+looseEndsMissingLinksSQL+`
		GROUP BY a.id
		HAVING COUNT(DISTINCT sa.show_id) >= ?
		ORDER BY `+looseEndsRotationSQL+`, a.id
		LIMIT ? OFFSET ?
	`, windowStart, windowEnd, chartingMinAppearances, viewerRotationKey(viewerID), rotationDay(), limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     fmt.Sprintf("%d recent show appearances, no Bandcamp or Spotify link", r.ShowCount),
			ShowCount:  r.ShowCount,
		})
	}
	return items, int64(total), nil
}

// --- Count helpers ---

func (s *DataQualityService) getCategoryCount(category string) (int, error) {
	var count int64
	var err error

	switch category {
	case "artists_missing_links":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM artists
			WHERE instagram IS NULL AND facebook IS NULL AND twitter IS NULL
			  AND youtube IS NULL AND spotify IS NULL AND soundcloud IS NULL
			  AND bandcamp IS NULL AND website IS NULL
		`).Scan(&count).Error

	case "artists_missing_location":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM artists
			WHERE city IS NULL AND state IS NULL
		`).Scan(&count).Error

	case "artists_no_aliases":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM artists a
			WHERE (SELECT COUNT(*) FROM show_artists WHERE artist_id = a.id) >= 5
			  AND (SELECT COUNT(*) FROM artist_aliases WHERE artist_id = a.id) = 0
		`).Scan(&count).Error

	case "venues_missing_social":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM venues
			WHERE instagram IS NULL AND facebook IS NULL AND twitter IS NULL
			  AND youtube IS NULL AND spotify IS NULL AND soundcloud IS NULL
			  AND bandcamp IS NULL AND website IS NULL
		`).Scan(&count).Error

	case "venues_unverified_with_shows":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM venues v
			WHERE v.verified = false
			  AND (SELECT COUNT(*) FROM show_venues sv
			       JOIN shows s ON s.id = sv.show_id AND s.status = 'approved'
			       WHERE sv.venue_id = v.id) >= 3
		`).Scan(&count).Error

	case "shows_no_billing_order":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM shows s
			WHERE s.status = 'approved' AND s.event_date >= NOW()
			  AND (SELECT COUNT(*) FROM show_artists WHERE show_id = s.id) >= 2
			  AND NOT EXISTS (
			    SELECT 1 FROM show_artists WHERE show_id = s.id AND position > 0
			  )
		`).Scan(&count).Error

	case "shows_missing_price":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM shows
			WHERE status = 'approved' AND event_date >= NOW() AND price IS NULL
		`).Scan(&count).Error

	case "releases_missing_year":
		err = s.db.Raw(`
			SELECT COUNT(*) FROM releases
			WHERE release_year IS NULL
		`).Scan(&count).Error
	}

	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// --- Item retrieval helpers ---

func (s *DataQualityService) getArtistsMissingLinks(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM artists
		WHERE instagram IS NULL AND facebook IS NULL AND twitter IS NULL
		  AND youtube IS NULL AND spotify IS NULL AND soundcloud IS NULL
		  AND bandcamp IS NULL AND website IS NULL
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	err = s.db.Raw(`
		SELECT a.id, a.name, a.slug, COUNT(sa.show_id) as show_count
		FROM artists a
		LEFT JOIN show_artists sa ON sa.artist_id = a.id
		LEFT JOIN shows s ON s.id = sa.show_id AND s.status = 'approved'
		WHERE a.instagram IS NULL AND a.facebook IS NULL AND a.twitter IS NULL
		  AND a.youtube IS NULL AND a.spotify IS NULL AND a.soundcloud IS NULL
		  AND a.bandcamp IS NULL AND a.website IS NULL
		GROUP BY a.id
		ORDER BY show_count DESC, a.name ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     "No social links or website",
			ShowCount:  r.ShowCount,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getArtistsMissingLocation(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM artists
		WHERE city IS NULL AND state IS NULL
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	err = s.db.Raw(`
		SELECT a.id, a.name, a.slug, COUNT(sa.show_id) as show_count
		FROM artists a
		LEFT JOIN show_artists sa ON sa.artist_id = a.id
		LEFT JOIN shows s ON s.id = sa.show_id AND s.status = 'approved'
		WHERE a.city IS NULL AND a.state IS NULL
		GROUP BY a.id
		ORDER BY show_count DESC, a.name ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     "No city or state set",
			ShowCount:  r.ShowCount,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getArtistsNoAliases(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM artists a
		WHERE (SELECT COUNT(*) FROM show_artists WHERE artist_id = a.id) >= 5
		  AND (SELECT COUNT(*) FROM artist_aliases WHERE artist_id = a.id) = 0
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	err = s.db.Raw(`
		SELECT a.id, a.name, a.slug, COUNT(sa.show_id) as show_count
		FROM artists a
		LEFT JOIN show_artists sa ON sa.artist_id = a.id
		WHERE (SELECT COUNT(*) FROM artist_aliases WHERE artist_id = a.id) = 0
		GROUP BY a.id
		HAVING COUNT(sa.show_id) >= 5
		ORDER BY show_count DESC, a.name ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "artist",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     fmt.Sprintf("%d shows but no aliases", r.ShowCount),
			ShowCount:  r.ShowCount,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getVenuesMissingSocial(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM venues
		WHERE instagram IS NULL AND facebook IS NULL AND twitter IS NULL
		  AND youtube IS NULL AND spotify IS NULL AND soundcloud IS NULL
		  AND bandcamp IS NULL AND website IS NULL
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	err = s.db.Raw(`
		SELECT v.id, v.name, v.slug, COUNT(sv.show_id) as show_count
		FROM venues v
		LEFT JOIN show_venues sv ON sv.venue_id = v.id
		LEFT JOIN shows s ON s.id = sv.show_id AND s.status = 'approved'
		WHERE v.instagram IS NULL AND v.facebook IS NULL AND v.twitter IS NULL
		  AND v.youtube IS NULL AND v.spotify IS NULL AND v.soundcloud IS NULL
		  AND v.bandcamp IS NULL AND v.website IS NULL
		GROUP BY v.id
		ORDER BY show_count DESC, v.name ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "venue",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     "No social links or website",
			ShowCount:  r.ShowCount,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getVenuesUnverifiedWithShows(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM venues v
		WHERE v.verified = false
		  AND (SELECT COUNT(*) FROM show_venues sv
		       JOIN shows s ON s.id = sv.show_id AND s.status = 'approved'
		       WHERE sv.venue_id = v.id) >= 3
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID        uint
		Name      string
		Slug      *string
		ShowCount int
	}
	var rows []row
	err = s.db.Raw(`
		SELECT v.id, v.name, v.slug, COUNT(sv.show_id) as show_count
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id AND s.status = 'approved'
		WHERE v.verified = false
		GROUP BY v.id
		HAVING COUNT(sv.show_id) >= 3
		ORDER BY show_count DESC, v.name ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "venue",
			EntityID:   r.ID,
			Name:       r.Name,
			Slug:       slug,
			Reason:     fmt.Sprintf("Unverified with %d approved shows", r.ShowCount),
			ShowCount:  r.ShowCount,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getShowsNoBillingOrder(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM shows s
		WHERE s.status = 'approved' AND s.event_date >= NOW()
		  AND (SELECT COUNT(*) FROM show_artists WHERE show_id = s.id) >= 2
		  AND NOT EXISTS (
		    SELECT 1 FROM show_artists WHERE show_id = s.id AND position > 0
		  )
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID    uint
		Title string
		Slug  *string
	}
	var rows []row
	err = s.db.Raw(`
		SELECT s.id, s.title, s.slug
		FROM shows s
		WHERE s.status = 'approved' AND s.event_date >= NOW()
		  AND (SELECT COUNT(*) FROM show_artists WHERE show_id = s.id) >= 2
		  AND NOT EXISTS (
		    SELECT 1 FROM show_artists WHERE show_id = s.id AND position > 0
		  )
		ORDER BY s.event_date ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "show",
			EntityID:   r.ID,
			Name:       r.Title,
			Slug:       slug,
			Reason:     "All artists at position 0 (no billing order)",
			ShowCount:  0,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getShowsMissingPrice(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM shows
		WHERE status = 'approved' AND event_date >= NOW() AND price IS NULL
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID    uint
		Title string
		Slug  *string
	}
	var rows []row
	err = s.db.Raw(`
		SELECT id, title, slug
		FROM shows
		WHERE status = 'approved' AND event_date >= NOW() AND price IS NULL
		ORDER BY event_date ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "show",
			EntityID:   r.ID,
			Name:       r.Title,
			Slug:       slug,
			Reason:     "No price set for upcoming show",
			ShowCount:  0,
		})
	}
	return items, total, nil
}

func (s *DataQualityService) getReleasesMissingYear(limit, offset int) ([]*contracts.DataQualityItem, int64, error) {
	var total int64
	err := s.db.Raw(`
		SELECT COUNT(*) FROM releases
		WHERE release_year IS NULL
	`).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	type row struct {
		ID          uint
		Title       string
		Slug        *string
		ReleaseType string
		ArtistNames string
	}
	var rows []row
	err = s.db.Raw(`
		SELECT r.id, r.title, r.slug, r.release_type,
		       COALESCE(string_agg(a.name, ', ' ORDER BY ar.position), '') as artist_names
		FROM releases r
		LEFT JOIN artist_releases ar ON ar.release_id = r.id
		LEFT JOIN artists a ON a.id = ar.artist_id
		WHERE r.release_year IS NULL
		GROUP BY r.id
		ORDER BY r.title ASC
		LIMIT ? OFFSET ?
	`, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*contracts.DataQualityItem, 0, len(rows))
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		reason := "No release year set"
		if r.ArtistNames != "" {
			reason = fmt.Sprintf("No release year set (by %s)", r.ArtistNames)
		}
		items = append(items, &contracts.DataQualityItem{
			EntityType: "release",
			EntityID:   r.ID,
			Name:       r.Title,
			Slug:       slug,
			Reason:     reason,
			ShowCount:  0,
		})
	}
	return items, total, nil
}
