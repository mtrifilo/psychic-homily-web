package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
)

// Default reminder check interval (30 minutes)
const DefaultReminderInterval = 30 * time.Minute

// reminderRow holds the result of the reminder query
type reminderRow struct {
	UserID    uint
	ShowID    uint
	Email     string
	ShowTitle string
	ShowSlug  string
	EventDate time.Time
}

// ReminderService sends email reminders for saved shows ~24h before the event
type ReminderService struct {
	db           *gorm.DB
	emailService EmailServiceInterface
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
	frontendURL  string
	jwtSecret    string
}

// NewReminderService creates a new reminder service
func NewReminderService(database *gorm.DB, emailService EmailServiceInterface, cfg *config.Config) *ReminderService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultReminderInterval

	if envInterval := os.Getenv("REMINDER_INTERVAL_MINUTES"); envInterval != "" {
		if minutes, err := strconv.Atoi(envInterval); err == nil && minutes > 0 {
			interval = time.Duration(minutes) * time.Minute
		}
	}

	return &ReminderService{
		db:           database,
		emailService: emailService,
		interval:     interval,
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
		frontendURL:  cfg.Email.FrontendURL,
		jwtSecret:    cfg.JWT.SecretKey,
	}
}

// Start begins the background reminder job
func (s *ReminderService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("show reminder service started",
		"interval_minutes", s.interval.Minutes(),
	)
}

// Stop gracefully stops the reminder service
func (s *ReminderService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("show reminder service stopped")
}

// run is the main loop for the reminder service
func (s *ReminderService) run(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on startup
	s.runReminderCycle()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("reminder service context cancelled")
			return
		case <-s.stopCh:
			s.logger.Info("reminder service received stop signal")
			return
		case <-ticker.C:
			s.runReminderCycle()
		}
	}
}

// runReminderCycle finds shows happening in ~24h and sends reminders
func (s *ReminderService) runReminderCycle() {
	s.logger.Info("starting show reminder cycle")

	now := time.Now()
	windowStart := now.Add(23 * time.Hour)
	windowEnd := now.Add(25 * time.Hour)

	// Query users with saved shows in the reminder window
	var rows []reminderRow
	err := s.db.Raw(`
		SELECT
			ub.user_id,
			ub.entity_id AS show_id,
			u.email,
			s.title AS show_title,
			COALESCE(s.slug, CAST(s.id AS TEXT)) AS show_slug,
			s.event_date
		FROM user_bookmarks ub
		JOIN shows s ON s.id = ub.entity_id
		JOIN users u ON u.id = ub.user_id
		JOIN user_preferences up ON up.user_id = ub.user_id
		WHERE ub.entity_type = 'show'
			AND ub.action = 'save'
			AND s.event_date BETWEEN ? AND ?
			AND s.status = 'approved'
			AND s.is_cancelled = false
			AND up.show_reminders = true
			AND u.is_active = true
			AND u.deleted_at IS NULL
			AND u.email IS NOT NULL
			AND ub.reminder_sent_at IS NULL
	`, windowStart, windowEnd).Scan(&rows).Error
	if err != nil {
		s.logger.Error("failed to query reminder candidates", "error", err)
		return
	}

	if len(rows) == 0 {
		s.logger.Info("no show reminders to send")
		return
	}

	s.logger.Info("found shows to send reminders for", "count", len(rows))

	// Collect venue names for each show
	type showVenueKey struct{ showID uint }
	venueCache := make(map[uint][]string)

	sentCount := 0
	errorCount := 0

	for _, row := range rows {
		// Fetch venues if not cached
		if _, ok := venueCache[row.ShowID]; !ok {
			var venueNames []string
			s.db.Raw(`
				SELECT v.name FROM venues v
				JOIN show_venues sv ON sv.venue_id = v.id
				WHERE sv.show_id = ?
			`, row.ShowID).Scan(&venueNames)
			venueCache[row.ShowID] = venueNames
		}

		showURL := fmt.Sprintf("%s/shows/%s", s.frontendURL, row.ShowSlug)
		unsubscribeURL := generateUnsubscribeURL(s.frontendURL, row.UserID, s.jwtSecret)

		err := s.emailService.SendShowReminderEmail(
			row.Email,
			row.ShowTitle,
			showURL,
			unsubscribeURL,
			row.EventDate,
			venueCache[row.ShowID],
		)
		if err != nil {
			s.logger.Error("failed to send show reminder email",
				"user_id", row.UserID,
				"show_id", row.ShowID,
				"error", err,
			)
			errorCount++
			continue
		}

		// Mark as sent for deduplication
		if err := s.db.Exec(
			"UPDATE user_bookmarks SET reminder_sent_at = ? WHERE user_id = ? AND entity_type = 'show' AND entity_id = ? AND action = 'save'",
			now, row.UserID, row.ShowID,
		).Error; err != nil {
			s.logger.Error("failed to mark reminder as sent",
				"user_id", row.UserID,
				"show_id", row.ShowID,
				"error", err,
			)
		}

		sentCount++
	}

	s.logger.Info("show reminder cycle completed",
		"total", len(rows),
		"sent", sentCount,
		"errors", errorCount,
	)
}

// RunReminderCycleNow triggers an immediate reminder cycle (useful for testing)
func (s *ReminderService) RunReminderCycleNow() {
	s.runReminderCycle()
}

// generateUnsubscribeURL creates an HMAC-signed URL for one-click unsubscribe
func generateUnsubscribeURL(baseURL string, userID uint, secret string) string {
	sig := computeUnsubscribeSignature(userID, secret)
	return fmt.Sprintf("%s/unsubscribe/show-reminders?uid=%d&sig=%s", baseURL, userID, sig)
}

// VerifyUnsubscribeSignature checks the HMAC signature for an unsubscribe request
func VerifyUnsubscribeSignature(userID uint, signature, secret string) bool {
	expected := computeUnsubscribeSignature(userID, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// computeUnsubscribeSignature computes HMAC-SHA256 of the user ID
func computeUnsubscribeSignature(userID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("unsubscribe:show-reminders:%d", userID)))
	return hex.EncodeToString(mac.Sum(nil))
}
