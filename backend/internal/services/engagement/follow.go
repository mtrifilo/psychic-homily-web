package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// FollowEntityUser is a pseudo-entity type used for user-to-user follow
// checks (e.g., comment reply-permission gating). It shares the
// user_bookmarks table but targets the users(id) space rather than an
// entity table.
const FollowEntityUser = "user"

// validFollowEntityTypes lists entity types that support following.
// Shows use save instead of follow.
var validFollowEntityTypes = map[string]bool{
	string(engagementm.BookmarkEntityArtist):    true,
	string(engagementm.BookmarkEntityVenue):     true,
	string(engagementm.BookmarkEntityLabel):     true,
	string(engagementm.BookmarkEntityFestival):  true,
	string(engagementm.BookmarkEntityScene):     true,
	string(engagementm.BookmarkEntityRadioShow): true,
	FollowEntityUser: true,
}

var libraryFollowEntityTypes = map[string]bool{
	string(engagementm.BookmarkEntityArtist):   true,
	string(engagementm.BookmarkEntityVenue):    true,
	string(engagementm.BookmarkEntityLabel):    true,
	string(engagementm.BookmarkEntityFestival): true,
	string(engagementm.BookmarkEntityScene):    true,
}

// FollowService handles follow/unfollow operations on entities.
// It wraps the generic user_bookmarks table with follow-specific logic.
type FollowService struct {
	db *gorm.DB
}

// NewFollowService creates a new follow service.
func NewFollowService(database *gorm.DB) *FollowService {
	if database == nil {
		database = db.GetDB()
	}
	return &FollowService{db: database}
}

// validateEntityType checks that entityType is a valid follow target.
func validateEntityType(entityType string) error {
	if !validFollowEntityTypes[entityType] {
		return apperrors.ErrFollowInvalidEntityType(entityType)
	}
	return nil
}

// Follow creates a follow bookmark. Idempotent — if already following, no error.
func (s *FollowService) Follow(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if err := validateEntityType(entityType); err != nil {
		return err
	}

	bookmark := engagementm.UserBookmark{
		UserID:     userID,
		EntityType: engagementm.BookmarkEntityType(entityType),
		EntityID:   entityID,
		Action:     engagementm.BookmarkActionFollow,
		CreatedAt:  time.Now().UTC(),
	}

	// ON CONFLICT DO NOTHING — idempotent under concurrent toggles. A plain
	// FirstOrCreate (SELECT-then-INSERT) lets two simultaneous follows both
	// miss the row and both INSERT, tripping the unique violation. The unique
	// constraint is user_bookmarks(user_id, entity_type, entity_id, action).
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&bookmark).Error; err != nil {
		return apperrors.ErrFollowInternal(err)
	}
	return nil
}

// Scene-follow notify modes (PSY-1341) — stored in user_bookmarks.settings
// under "scene_notify_mode". Absent settings mean SceneNotifyModeAll.
const (
	SceneNotifyModeAll           = "all"
	SceneNotifyModeFollowedBands = "followed_bands_only"
	sceneNotifyModeKey           = "scene_notify_mode"
)

// SetSceneNotifyMode stores the scene follow's new-show notification mode on
// the existing follow row. The follow must already exist (call Follow first);
// a missing row is an error so a client can't silently configure nothing.
func (s *FollowService) SetSceneNotifyMode(userID uint, sceneID uint, mode string) error {
	if s.db == nil {
		return apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if mode != SceneNotifyModeAll && mode != SceneNotifyModeFollowedBands {
		return apperrors.ErrFollowInvalidEntityType(fmt.Sprintf("invalid notify mode: %s", mode))
	}
	res := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, engagementm.BookmarkEntityScene, sceneID, engagementm.BookmarkActionFollow).
		Update("settings", gorm.Expr(
			`COALESCE(settings, '{}'::jsonb) || jsonb_build_object(?::text, ?::text)`,
			sceneNotifyModeKey, mode))
	if res.Error != nil {
		return apperrors.ErrFollowInternal(fmt.Errorf("failed to set notify mode: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrFollowInternal(fmt.Errorf("no scene follow to configure"))
	}
	return nil
}

// SceneNotifyMode reads the stored mode for a user's scene follow; absent
// settings (or no follow) return SceneNotifyModeAll.
func (s *FollowService) SceneNotifyMode(userID uint, sceneID uint) (string, error) {
	if s.db == nil {
		return "", apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	// Pluck requires a slice destination; COALESCE folds the two "no explicit
	// mode" shapes (NULL settings, absent key) into '' so zero rows (no
	// follow) and unset settings both read as the default.
	var modes []string
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, engagementm.BookmarkEntityScene, sceneID, engagementm.BookmarkActionFollow).
		Pluck(`COALESCE(settings->>'scene_notify_mode', '')`, &modes).Error
	if err != nil {
		return "", apperrors.ErrFollowInternal(fmt.Errorf("failed to read notify mode: %w", err))
	}
	if len(modes) == 0 || modes[0] == "" {
		return SceneNotifyModeAll, nil
	}
	return modes[0], nil
}

// Unfollow removes a follow bookmark. Idempotent — if not following, no error.
func (s *FollowService) Unfollow(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if err := validateEntityType(entityType); err != nil {
		return err
	}

	result := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow,
	).Delete(&engagementm.UserBookmark{})

	if result.Error != nil {
		return apperrors.ErrFollowInternal(result.Error)
	}
	return nil
}

// IsFollowing checks if a user follows a specific entity.
func (s *FollowService) IsFollowing(userID uint, entityType string, entityID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return false, err
	}

	var count int64
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check follow status: %w", err)
	}
	return count > 0, nil
}

// GetFollowerCount returns the number of followers for a specific entity.
func (s *FollowService) GetFollowerCount(entityType string, entityID uint) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return 0, err
	}

	var count int64
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get follower count: %w", err)
	}
	return count, nil
}

// GetBatchFollowerCounts returns follower counts for multiple entities of the same type.
func (s *FollowService) GetBatchFollowerCounts(entityType string, entityIDs []uint) (map[uint]int64, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, err
	}

	result := make(map[uint]int64)
	if len(entityIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		EntityID uint
		Count    int64
	}
	var rows []countRow

	err := s.db.Model(&engagementm.UserBookmark{}).
		Select("entity_id, COUNT(*) as count").
		Where("entity_type = ? AND entity_id IN ? AND action = ?",
			engagementm.BookmarkEntityType(entityType), entityIDs, engagementm.BookmarkActionFollow).
		Group("entity_id").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch follower counts: %w", err)
	}

	// Initialize all requested IDs with 0
	for _, id := range entityIDs {
		result[id] = 0
	}
	for _, row := range rows {
		result[row.EntityID] = row.Count
	}
	return result, nil
}

// GetBatchUserFollowing returns which entities the user follows from a list.
func (s *FollowService) GetBatchUserFollowing(userID uint, entityType string, entityIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, err
	}

	result := make(map[uint]bool)
	if len(entityIDs) == 0 {
		return result, nil
	}

	var bookmarks []engagementm.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id IN ? AND action = ?",
		userID, engagementm.BookmarkEntityType(entityType), entityIDs, engagementm.BookmarkActionFollow,
	).Find(&bookmarks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch user following: %w", err)
	}

	for _, b := range bookmarks {
		result[b.EntityID] = true
	}
	return result, nil
}

// GetUserFollowing lists all entities a user follows of a given type, ordered by follow date DESC.
// If entityType is empty, returns follows across all types.
func (s *FollowService) GetUserFollowing(userID uint, entityType string, limit, offset int) ([]*contracts.FollowingEntityResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if entityType != "" {
		if err := validateEntityType(entityType); err != nil {
			return nil, 0, err
		}
	}

	// Build base condition
	baseQuery := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND action = ?", userID, engagementm.BookmarkActionFollow)
	if entityType != "" {
		baseQuery = baseQuery.Where("entity_type = ?", engagementm.BookmarkEntityType(entityType))
	}

	// Count total
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user following: %w", err)
	}
	if total == 0 {
		return []*contracts.FollowingEntityResponse{}, 0, nil
	}

	// Get bookmarks
	var bookmarks []engagementm.UserBookmark
	if err := baseQuery.Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&bookmarks).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user following: %w", err)
	}

	// Group bookmarks by entity type for batch name/slug lookups
	type entityKey struct {
		Type string
		ID   uint
	}
	entityNames := make(map[entityKey]struct{ Name, Slug string })

	// radioEnriched holds the radio-show-only fields (PSY-1356) that don't fit
	// the shared {Name, Slug} shape. Keyed by radio_show id; populated only when
	// the followed set includes radio shows.
	type radioFollowFields struct {
		StationName     string
		StationSlug     string
		HostName        *string
		LastEpisodeDate *string
	}
	radioEnriched := make(map[uint]radioFollowFields)

	// Collect IDs by type
	idsByType := make(map[string][]uint)
	for _, b := range bookmarks {
		t := string(b.EntityType)
		idsByType[t] = append(idsByType[t], b.EntityID)
	}

	// Batch lookup for each type
	for t, ids := range idsByType {
		switch t {
		case string(engagementm.BookmarkEntityArtist):
			var artists []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("artists").Select("id, name, slug").Where("id IN ?", ids).Find(&artists)
			for _, a := range artists {
				slug := ""
				if a.Slug != nil {
					slug = *a.Slug
				}
				entityNames[entityKey{t, a.ID}] = struct{ Name, Slug string }{a.Name, slug}
			}
		case string(engagementm.BookmarkEntityVenue):
			var venues []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("venues").Select("id, name, slug").Where("id IN ?", ids).Find(&venues)
			for _, v := range venues {
				slug := ""
				if v.Slug != nil {
					slug = *v.Slug
				}
				entityNames[entityKey{t, v.ID}] = struct{ Name, Slug string }{v.Name, slug}
			}
		case string(engagementm.BookmarkEntityLabel):
			var labels []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("labels").Select("id, name, slug").Where("id IN ?", ids).Find(&labels)
			for _, l := range labels {
				slug := ""
				if l.Slug != nil {
					slug = *l.Slug
				}
				entityNames[entityKey{t, l.ID}] = struct{ Name, Slug string }{l.Name, slug}
			}
		case string(engagementm.BookmarkEntityScene):
			var scenes []struct {
				ID    uint
				City  string
				State string
				Slug  string
			}
			s.db.Table("scenes").Select("id, city, state, slug").Where("id IN ?", ids).Find(&scenes)
			for _, sc := range scenes {
				entityNames[entityKey{t, sc.ID}] = struct{ Name, Slug string }{
					fmt.Sprintf("%s, %s", sc.City, sc.State), sc.Slug,
				}
			}
		case string(engagementm.BookmarkEntityFestival):
			var festivals []struct {
				ID   uint
				Name string
				Slug string
			}
			s.db.Table("festivals").Select("id, name, slug").Where("id IN ?", ids).Find(&festivals)
			for _, f := range festivals {
				entityNames[entityKey{t, f.ID}] = struct{ Name, Slug string }{f.Name, f.Slug}
			}
		case string(engagementm.BookmarkEntityRadioShow):
			// One query: show ⋈ station, plus a correlated latest-episode lookup
			// (idx_radio_episodes_show is (show_id, air_date DESC), so the
			// ORDER BY … LIMIT 1 is an index seek — no N+1 per followed show).
			// station_id is NOT NULL, so the inner join never drops a live show.
			//
			// last_episode_date = the show's most recent AIRED episode, gated on
			// the STATION-LOCAL date, NOT the DB's UTC CURRENT_DATE. WFMU-style
			// providers pre-publish an episode row dated for the station's local
			// "tomorrow", and every onboarded station runs at <= 0 UTC offset, so
			// a bare UTC gate would surface that not-yet-aired row as "last-aired"
			// every evening (the PSY-1374 footgun). The tz expression mirrors
			// catalog.stationLocalToday: pg_timezone_names resolves st.timezone
			// case-insensitively and COALESCEs an unknown/NULL zone to UTC, so a
			// bad tz string can't error the query. Residual (accepted,
			// date-granularity): an episode airing LATER today still shows today's
			// date — excluding a windowed-not-yet-aired same-day row would need
			// airedEpisodeVisibleSQL's starts_at/play_count checks, which are
			// single-station-scoped and don't fit this multi-station query. See
			// PSY-1356 for the aired-gate decision. to_char pins YYYY-MM-DD (a raw
			// date scan can arrive as an RFC3339 timestamp; see catalog.normalizeDate).
			var shows []struct {
				ID              uint
				Name            string
				Slug            string
				HostName        *string
				StationName     string
				StationSlug     string
				LastEpisodeDate *string
			}
			if err := s.db.Table("radio_shows AS rs").
				Select(`rs.id, rs.name, rs.slug, rs.host_name,
					st.name AS station_name, st.slug AS station_slug,
					(SELECT to_char(e.air_date, 'YYYY-MM-DD') FROM radio_episodes e
					 WHERE e.show_id = rs.id
					   AND e.air_date <= (now() AT TIME ZONE COALESCE(
					         (SELECT name FROM pg_timezone_names
					          WHERE lower(name) = lower(btrim(st.timezone, E' \t\n\r'))),
					         'UTC'))::date
					 ORDER BY e.air_date DESC LIMIT 1) AS last_episode_date`).
				Joins("JOIN radio_stations st ON st.id = rs.station_id").
				Where("rs.id IN ?", ids).
				Find(&shows).Error; err != nil {
				// Best-effort enrichment, like the sibling per-type lookups above —
				// but this JOIN + correlated tz subquery is the most failure-prone
				// of them, so LOG rather than degrade silently to blank rows on a
				// schema/SQL regression (the following list still renders).
				logger.Default().Error("get_user_following_radio_enrich_failed",
					"user_id", userID, "show_id_count", len(ids), "error", err.Error())
			}
			for _, sh := range shows {
				entityNames[entityKey{t, sh.ID}] = struct{ Name, Slug string }{sh.Name, sh.Slug}
				radioEnriched[sh.ID] = radioFollowFields{
					StationName:     sh.StationName,
					StationSlug:     sh.StationSlug,
					HostName:        sh.HostName,
					LastEpisodeDate: sh.LastEpisodeDate,
				}
			}
		}
	}

	// Build response
	responses := make([]*contracts.FollowingEntityResponse, 0, len(bookmarks))
	for _, b := range bookmarks {
		info := entityNames[entityKey{string(b.EntityType), b.EntityID}]
		resp := &contracts.FollowingEntityResponse{
			EntityType: string(b.EntityType),
			EntityID:   b.EntityID,
			Name:       info.Name,
			Slug:       info.Slug,
			FollowedAt: b.CreatedAt,
		}
		// Attach radio-show-only enriched fields (PSY-1356). rf is a fresh copy
		// each iteration, so taking its field addresses is safe.
		if b.EntityType == engagementm.BookmarkEntityRadioShow {
			if rf, ok := radioEnriched[b.EntityID]; ok {
				resp.StationName = &rf.StationName
				resp.StationSlug = &rf.StationSlug
				resp.HostName = rf.HostName
				resp.LastEpisodeDate = rf.LastEpisodeDate
			}
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}

// GetLibraryFollowingCounts returns all Library follow-tab totals in one query.
func (s *FollowService) GetLibraryFollowingCounts(userID uint) (*contracts.LibraryFollowingCounts, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	counts := &contracts.LibraryFollowingCounts{}
	if err := s.db.Table("user_bookmarks AS ub").
		Select(`
			COUNT(*) FILTER (WHERE ub.entity_type = 'artist' AND EXISTS (SELECT 1 FROM artists e WHERE e.id = ub.entity_id)) AS artists,
			COUNT(*) FILTER (WHERE ub.entity_type = 'venue' AND EXISTS (SELECT 1 FROM venues e WHERE e.id = ub.entity_id)) AS venues,
			COUNT(*) FILTER (WHERE ub.entity_type = 'scene' AND EXISTS (SELECT 1 FROM scenes e WHERE e.id = ub.entity_id)) AS scenes,
			COUNT(*) FILTER (WHERE ub.entity_type = 'label' AND EXISTS (SELECT 1 FROM labels e WHERE e.id = ub.entity_id)) AS labels,
			COUNT(*) FILTER (WHERE ub.entity_type = 'festival' AND EXISTS (SELECT 1 FROM festivals e WHERE e.id = ub.entity_id)) AS festivals`).
		Where("ub.user_id = ? AND ub.action = ?", userID, engagementm.BookmarkActionFollow).
		Scan(counts).Error; err != nil {
		return nil, fmt.Errorf("failed to count library following: %w", err)
	}
	return counts, nil
}

type libraryFollowingSource struct {
	table          string
	alias          string
	nameExpression string
	slugExpression string
}

func libraryFollowingSourceFor(entityType string) (libraryFollowingSource, error) {
	switch entityType {
	case string(engagementm.BookmarkEntityArtist):
		return libraryFollowingSource{"artists", "entity", "entity.name", "COALESCE(entity.slug, '')"}, nil
	case string(engagementm.BookmarkEntityVenue):
		return libraryFollowingSource{"venues", "entity", "entity.name", "COALESCE(entity.slug, '')"}, nil
	case string(engagementm.BookmarkEntityScene):
		return libraryFollowingSource{"scenes", "entity", "entity.city || ', ' || entity.state", "entity.slug"}, nil
	case string(engagementm.BookmarkEntityLabel):
		return libraryFollowingSource{"labels", "entity", "entity.name", "COALESCE(entity.slug, '')"}, nil
	case string(engagementm.BookmarkEntityFestival):
		return libraryFollowingSource{"festivals", "entity", "entity.name", "entity.slug"}, nil
	default:
		return libraryFollowingSource{}, fmt.Errorf("entity type %q is not available in Library", entityType)
	}
}

// GetLibraryFollowing returns one bounded, globally alphabetical Library page.
// Keyset pagination over the complete sort tuple remains stable when follows
// are added or removed before a previously loaded page boundary.
func (s *FollowService) GetLibraryFollowing(userID uint, entityType string, limit int, cursor *contracts.LibraryFollowingCursor) ([]*contracts.FollowingEntityResponse, *contracts.LibraryFollowingCursor, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}
	if limit < 1 || limit > 100 {
		return nil, nil, fmt.Errorf("library following limit must be between 1 and 100")
	}
	if !libraryFollowEntityTypes[entityType] {
		return nil, nil, fmt.Errorf("entity type %q is not available in Library", entityType)
	}

	source, err := libraryFollowingSourceFor(entityType)
	if err != nil {
		return nil, nil, err
	}

	base := s.db.Table("user_bookmarks AS ub").
		Joins(fmt.Sprintf("JOIN %s AS %s ON %s.id = ub.entity_id", source.table, source.alias, source.alias)).
		Where("ub.user_id = ? AND ub.action = ? AND ub.entity_type = ?", userID, engagementm.BookmarkActionFollow, entityType)
	if cursor != nil {
		boundarySQL := fmt.Sprintf("(LOWER(%s), %s, ub.entity_id) > (?, ?, ?)", source.nameExpression, source.nameExpression)
		base = base.Where(boundarySQL, cursor.SortName, cursor.Name, cursor.EntityID)
	}

	type libraryFollowingRow struct {
		EntityType string
		EntityID   uint
		Name       string
		Slug       string
		FollowedAt time.Time
		SortName   string
	}
	var rows []libraryFollowingRow
	selectSQL := fmt.Sprintf("ub.entity_type, ub.entity_id, %s AS name, %s AS slug, ub.created_at AS followed_at, LOWER(%s) AS sort_name", source.nameExpression, source.slugExpression, source.nameExpression)
	orderSQL := fmt.Sprintf("LOWER(%s) ASC, %s ASC, ub.entity_id ASC", source.nameExpression, source.nameExpression)
	if err := base.Select(selectSQL).Order(orderSQL).Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get library following: %w", err)
	}

	var nextCursor *contracts.LibraryFollowingCursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		nextCursor = &contracts.LibraryFollowingCursor{
			SortName: last.SortName,
			Name:     last.Name,
			EntityID: last.EntityID,
		}
	}

	following := make([]*contracts.FollowingEntityResponse, 0, len(rows))
	for _, row := range rows {
		following = append(following, &contracts.FollowingEntityResponse{
			EntityType: row.EntityType,
			EntityID:   row.EntityID,
			Name:       row.Name,
			Slug:       row.Slug,
			FollowedAt: row.FollowedAt,
		})
	}
	return following, nextCursor, nil
}

// GetFollowers lists followers of an entity. Returns user info.
func (s *FollowService) GetFollowers(entityType string, entityID uint, limit, offset int) ([]*contracts.FollowerResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, 0, err
	}

	// Count total followers
	var total int64
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count followers: %w", err)
	}
	if total == 0 {
		return []*contracts.FollowerResponse{}, 0, nil
	}

	// Query bookmarks joined with users
	type followerRow struct {
		UserID      uint
		Username    *string
		DisplayName *string
	}
	var rows []followerRow

	err = s.db.Table("user_bookmarks").
		// display_name falls back to first_name for rows predating PSY-1063
		// (this hand-rolled projection predates the shared resolver; NULLIF
		// keeps a cleared display_name from masking the fallback).
		Select("user_bookmarks.user_id, users.username, COALESCE(NULLIF(users.display_name, ''), users.first_name) as display_name").
		Joins("JOIN users ON users.id = user_bookmarks.user_id").
		Where("user_bookmarks.entity_type = ? AND user_bookmarks.entity_id = ? AND user_bookmarks.action = ?",
			engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
		Where("users.deleted_at IS NULL").
		Order("user_bookmarks.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get followers: %w", err)
	}

	responses := make([]*contracts.FollowerResponse, 0, len(rows))
	for _, row := range rows {
		resp := &contracts.FollowerResponse{
			UserID: row.UserID,
		}
		if row.Username != nil {
			resp.Username = *row.Username
		}
		if row.DisplayName != nil {
			resp.DisplayName = *row.DisplayName
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}
