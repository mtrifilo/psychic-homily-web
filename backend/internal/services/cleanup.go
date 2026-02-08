package services

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
)

// Default cleanup interval (24 hours)
const DefaultCleanupInterval = 24 * time.Hour

// CleanupService handles background cleanup tasks
type CleanupService struct {
	db           *gorm.DB
	userService  *UserService
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(database *gorm.DB) *CleanupService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultCleanupInterval

	// Allow override via environment variable (for testing)
	if envInterval := os.Getenv("CLEANUP_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	return &CleanupService{
		db:          database,
		userService: NewUserService(database),
		interval:    interval,
		stopCh:      make(chan struct{}),
		logger:      slog.Default(),
	}
}

// Start begins the background cleanup job
func (s *CleanupService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("account cleanup service started",
		"interval_hours", s.interval.Hours(),
	)
}

// Stop gracefully stops the cleanup service
func (s *CleanupService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("account cleanup service stopped")
}

// run is the main loop for the cleanup service
func (s *CleanupService) run(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on startup
	s.runCleanupCycle()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

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
			"email_hash", hashEmail(email),
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
