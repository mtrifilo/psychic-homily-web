package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/notification"
)

// Default cleanup interval (24 hours)
const DefaultCleanupInterval = 24 * time.Hour

// Tag prune defaults (Gazelle-style downvote auto-prune for entity_tags).
const (
	DefaultTagPruneInterval = 1 * time.Hour
	// Strict inequality on downs > ups — ties stay. Gazelle precedent requires ≥2 downs.
	MinDownvotesToPrune = 2
)

// Audit log action name for tag prune cycles (system-initiated, ActorID nil).
const AuditActionPruneDownvotedTags = "prune_downvoted_tags"

// cleanupUserService is the minimal interface CleanupService needs from UserService.
type cleanupUserService interface {
	GetExpiredDeletedAccounts() ([]models.User, error)
	PermanentlyDeleteUser(userID uint) error
}

// CleanupService handles background cleanup tasks
type CleanupService struct {
	db               *gorm.DB
	userService      cleanupUserService
	interval         time.Duration
	tagPruneInterval time.Duration
	tagPruneEnabled  bool
	tagPruneDryRun   bool
	stopCh           chan struct{}
	wg               sync.WaitGroup
	logger           *slog.Logger
}

// NewCleanupService creates a new cleanup service.
// userSvc must implement GetExpiredDeletedAccounts and PermanentlyDeleteUser.
func NewCleanupService(database *gorm.DB, userSvc cleanupUserService) *CleanupService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultCleanupInterval
	if envInterval := os.Getenv("CLEANUP_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	tagPruneInterval := DefaultTagPruneInterval
	if envInterval := os.Getenv("TAG_PRUNE_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			tagPruneInterval = time.Duration(hours) * time.Hour
		}
	}

	tagPruneEnabled := true
	if v := os.Getenv("TAG_PRUNE_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			tagPruneEnabled = parsed
		}
	}

	tagPruneDryRun := false
	if v := os.Getenv("TAG_PRUNE_DRY_RUN"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			tagPruneDryRun = parsed
		}
	}

	return &CleanupService{
		db:               database,
		userService:      userSvc,
		interval:         interval,
		tagPruneInterval: tagPruneInterval,
		tagPruneEnabled:  tagPruneEnabled,
		tagPruneDryRun:   tagPruneDryRun,
		stopCh:           make(chan struct{}),
		logger:           slog.Default(),
	}
}

// Start begins the background cleanup job
func (s *CleanupService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("account cleanup service started",
		"interval_hours", s.interval.Hours(),
		"tag_prune_enabled", s.tagPruneEnabled,
		"tag_prune_dry_run", s.tagPruneDryRun,
		"tag_prune_interval_hours", s.tagPruneInterval.Hours(),
	)
}

// Stop gracefully stops the cleanup service
func (s *CleanupService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("account cleanup service stopped")
}

// run is the main loop for the cleanup service.
// Runs account cleanup and tag prune on independent tickers in a single goroutine.
func (s *CleanupService) run(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on startup
	s.runCleanupCycle()
	s.runTagPruneCycle(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	tagPruneTicker := time.NewTicker(s.tagPruneInterval)
	defer tagPruneTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("cleanup service context cancelled")
			return
		case <-s.stopCh:
			s.logger.Info("cleanup service received stop signal")
			return
		case <-ticker.C:
			s.runCleanupCycle()
		case <-tagPruneTicker.C:
			s.runTagPruneCycle(ctx)
		}
	}
}

// runCleanupCycle performs a single cleanup cycle
func (s *CleanupService) runCleanupCycle() {
	s.logger.Info("starting account cleanup cycle")

	expiredAccounts, err := s.userService.GetExpiredDeletedAccounts()
	if err != nil {
		s.logger.Error("failed to get expired deleted accounts",
			"error", err,
		)
		return
	}

	if len(expiredAccounts) == 0 {
		s.logger.Info("no expired accounts to purge")
		return
	}

	s.logger.Info("found expired accounts to purge",
		"count", len(expiredAccounts),
	)

	purgedCount := 0
	for _, account := range expiredAccounts {
		// Log before purging
		email := ""
		if account.Email != nil {
			email = *account.Email
		}
		deletedAt := ""
		if account.DeletedAt != nil {
			deletedAt = account.DeletedAt.Format(time.RFC3339)
		}

		s.logger.Info("purging expired account",
			"user_id", account.ID,
			"email_hash", notification.HashEmail(email),
			"deleted_at", deletedAt,
		)

		if err := s.userService.PermanentlyDeleteUser(account.ID); err != nil {
			s.logger.Error("failed to permanently delete user",
				"user_id", account.ID,
				"error", err,
			)
			continue
		}

		purgedCount++
		s.logger.Info("successfully purged account",
			"user_id", account.ID,
		)
	}

	s.logger.Info("account cleanup cycle completed",
		"total_expired", len(expiredAccounts),
		"purged", purgedCount,
		"failed", len(expiredAccounts)-purgedCount,
	)
}

// RunCleanupNow triggers an immediate cleanup cycle (useful for testing)
func (s *CleanupService) RunCleanupNow() {
	s.runCleanupCycle()
}

// RunTagPruneNow triggers an immediate tag prune cycle (useful for testing).
// Returns the number of entity_tags rows deleted (0 in dry-run).
func (s *CleanupService) RunTagPruneNow(ctx context.Context) (int64, error) {
	return s.pruneDownvotedEntityTags(ctx)
}

// runTagPruneCycle performs a single tag prune cycle, writing an audit log entry.
// Audit log errors are logged but never fail the cycle (fire-and-forget).
func (s *CleanupService) runTagPruneCycle(ctx context.Context) {
	if !s.tagPruneEnabled {
		s.logger.Info("tag prune skipped: disabled via TAG_PRUNE_ENABLED")
		return
	}

	s.logger.Info("starting tag prune cycle",
		"dry_run", s.tagPruneDryRun,
		"threshold_min_downs", MinDownvotesToPrune,
	)

	deleted, err := s.pruneDownvotedEntityTags(ctx)
	if err != nil {
		s.logger.Error("tag prune cycle failed", "error", err)
		return
	}

	s.logger.Info("tag prune cycle completed",
		"deleted_count", deleted,
		"dry_run", s.tagPruneDryRun,
	)

	s.writeTagPruneAuditLog(deleted)
}

// pruneDownvotedEntityTags deletes entity_tags rows whose per-entity vote
// aggregate has downs > ups AND downs >= MinDownvotesToPrune.
// In dry-run mode, returns the count of rows that would be deleted without deleting.
// Only removes the entity-tag application — the tag row in `tags` is untouched.
func (s *CleanupService) pruneDownvotedEntityTags(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, nil
	}

	// Strict `v.downs > v.ups` so ties stay; `v.downs >= ?` enforces the minimum.
	selectSQL := `
		SELECT et.id
		FROM entity_tags et
		JOIN (
			SELECT tag_id, entity_type, entity_id,
			       SUM(CASE WHEN vote = 1 THEN 1 ELSE 0 END) AS ups,
			       SUM(CASE WHEN vote = -1 THEN 1 ELSE 0 END) AS downs
			FROM tag_votes
			GROUP BY tag_id, entity_type, entity_id
		) v ON v.tag_id = et.tag_id
		   AND v.entity_type = et.entity_type
		   AND v.entity_id = et.entity_id
		WHERE v.downs > v.ups AND v.downs >= ?
	`

	if s.tagPruneDryRun {
		var count int64
		if err := s.db.WithContext(ctx).
			Raw("SELECT COUNT(*) FROM ("+selectSQL+") sub", MinDownvotesToPrune).
			Scan(&count).Error; err != nil {
			return 0, err
		}
		return count, nil
	}

	result := s.db.WithContext(ctx).Exec(
		"DELETE FROM entity_tags WHERE id IN ("+selectSQL+")",
		MinDownvotesToPrune,
	)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// writeTagPruneAuditLog records a tag prune cycle summary in the audit log.
// Fire-and-forget — errors log but never fail the parent operation.
func (s *CleanupService) writeTagPruneAuditLog(deleted int64) {
	if s.db == nil {
		return
	}

	metadata := map[string]interface{}{
		"deleted_count":       deleted,
		"threshold_min_downs": MinDownvotesToPrune,
		"dry_run":             s.tagPruneDryRun,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Error("failed to marshal tag prune audit log metadata", "error", err)
		return
	}

	raw := json.RawMessage(metadataJSON)
	auditLog := models.AuditLog{
		ActorID:    nil,
		Action:     AuditActionPruneDownvotedTags,
		EntityType: "entity_tags",
		EntityID:   0,
		Metadata:   &raw,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&auditLog).Error; err != nil {
		s.logger.Error("failed to write tag prune audit log", "error", err)
	}
}

// hashEmail masks an email for privacy (e.g., "jo***@example.com")
func hashEmail(email string) string {
	if email == "" {
		return "N/A"
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "N/A"
	}

	local := parts[0]
	domain := parts[1]

	if len(local) <= 2 {
		return local[:1] + "***@" + domain
	}

	return local[:2] + "***@" + domain
}
