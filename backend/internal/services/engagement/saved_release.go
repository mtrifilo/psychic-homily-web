package engagement

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// SavedReleaseService owns the user-facing Save/Saved relationship for
// releases. The underlying action remains `bookmark` for compatibility with
// existing rows and the legacy hot-releases chart; callers never depend on
// that storage detail.
type SavedReleaseService struct {
	db             *gorm.DB
	bookmark       *BookmarkService
	releaseService contracts.ReleaseServiceInterface
}

func NewSavedReleaseService(database *gorm.DB, releaseService contracts.ReleaseServiceInterface) *SavedReleaseService {
	if database == nil {
		database = db.GetDB()
	}
	return &SavedReleaseService{
		db:             database,
		bookmark:       NewBookmarkService(database),
		releaseService: releaseService,
	}
}

func (s *SavedReleaseService) SaveRelease(userID, releaseID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var release catalogm.Release
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").First(&release, releaseID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrReleaseNotFound(releaseID)
			}
			return fmt.Errorf("failed to verify release: %w", err)
		}

		if err := NewBookmarkService(tx).CreateBookmark(
			userID,
			engagementm.BookmarkEntityRelease,
			releaseID,
			engagementm.BookmarkActionReleaseSave,
		); err != nil {
			return fmt.Errorf("failed to save release: %w", err)
		}
		return nil
	})
}

func (s *SavedReleaseService) UnsaveRelease(userID, releaseID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	err := s.bookmark.DeleteBookmark(
		userID,
		engagementm.BookmarkEntityRelease,
		releaseID,
		engagementm.BookmarkActionReleaseSave,
	)
	if err != nil {
		if err.Error() == "bookmark not found" {
			return fmt.Errorf("release was not saved")
		}
		return fmt.Errorf("failed to unsave release: %w", err)
	}
	return nil
}

type savedReleaseRef struct {
	ReleaseID uint      `gorm:"column:release_id"`
	SavedAt   time.Time `gorm:"column:saved_at"`
}

// GetUserSavedReleases returns the user's saved releases newest-save first.
// The join excludes any historical dangling polymorphic rows, and hydration
// uses one batched catalog call before restoring bookmark order.
func (s *SavedReleaseService) GetUserSavedReleases(userID uint, limit, offset int) ([]*contracts.SavedReleaseResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if s.releaseService == nil {
		return nil, 0, fmt.Errorf("release service not initialized")
	}

	baseQuery := func() *gorm.DB {
		return s.db.Table("user_bookmarks").
			Joins("JOIN releases ON releases.id = user_bookmarks.entity_id").
			Where(
				"user_bookmarks.user_id = ? AND user_bookmarks.entity_type = ? AND user_bookmarks.action = ?",
				userID,
				engagementm.BookmarkEntityRelease,
				engagementm.BookmarkActionReleaseSave,
			)
	}

	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count saved releases: %w", err)
	}

	var refs []savedReleaseRef
	err := baseQuery().
		Select("user_bookmarks.entity_id AS release_id, user_bookmarks.created_at AS saved_at").
		Order("user_bookmarks.created_at DESC, user_bookmarks.id DESC").
		Limit(limit).
		Offset(offset).
		Find(&refs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get saved releases: %w", err)
	}
	if len(refs) == 0 {
		return []*contracts.SavedReleaseResponse{}, total, nil
	}

	releaseIDs := make([]uint, len(refs))
	for i, ref := range refs {
		releaseIDs[i] = ref.ReleaseID
	}
	releases, err := s.releaseService.GetReleasesByIDs(releaseIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to hydrate saved releases: %w", err)
	}

	releasesByID := make(map[uint]*contracts.ReleaseListResponse, len(releases))
	for _, release := range releases {
		releasesByID[release.ID] = release
	}

	responses := make([]*contracts.SavedReleaseResponse, 0, len(refs))
	for _, ref := range refs {
		if release, ok := releasesByID[ref.ReleaseID]; ok {
			responses = append(responses, &contracts.SavedReleaseResponse{
				ReleaseListResponse: *release,
				SavedAt:             ref.SavedAt,
			})
		}
	}
	return responses, total, nil
}

func (s *SavedReleaseService) IsReleaseSaved(userID, releaseID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	return s.bookmark.IsBookmarked(
		userID,
		engagementm.BookmarkEntityRelease,
		releaseID,
		engagementm.BookmarkActionReleaseSave,
	)
}

func (s *SavedReleaseService) GetSavedReleaseIDs(userID uint, releaseIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return s.bookmark.GetBookmarkedEntityIDs(
		userID,
		engagementm.BookmarkEntityRelease,
		engagementm.BookmarkActionReleaseSave,
		releaseIDs,
	)
}

func (s *SavedReleaseService) GetSaveCount(releaseID uint) (int, error) {
	counts, err := s.GetBatchSaveCounts([]uint{releaseID})
	if err != nil {
		return 0, err
	}
	return counts[releaseID], nil
}

// GetBatchSaveCounts returns one zero-filled public count per requested ID.
// Joining releases makes old dangling polymorphic rows invisible without
// turning the endpoint into an existence oracle.
func (s *SavedReleaseService) GetBatchSaveCounts(releaseIDs []uint) (map[uint]int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]int, len(releaseIDs))
	for _, id := range releaseIDs {
		result[id] = 0
	}
	if len(releaseIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		EntityID uint `gorm:"column:entity_id"`
		Count    int  `gorm:"column:count"`
	}
	var rows []countRow
	err := s.db.Model(&engagementm.UserBookmark{}).
		Select("user_bookmarks.entity_id, COUNT(*) AS count").
		Joins("JOIN releases ON releases.id = user_bookmarks.entity_id").
		Where(
			"user_bookmarks.entity_type = ? AND user_bookmarks.entity_id IN ? AND user_bookmarks.action = ?",
			engagementm.BookmarkEntityRelease,
			releaseIDs,
			engagementm.BookmarkActionReleaseSave,
		).
		Group("user_bookmarks.entity_id").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch release save counts: %w", err)
	}
	for _, row := range rows {
		if _, requested := result[row.EntityID]; requested {
			result[row.EntityID] = row.Count
		}
	}
	return result, nil
}
