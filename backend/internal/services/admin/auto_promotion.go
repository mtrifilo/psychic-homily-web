package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/shared"
)

// User tier constants.
const (
	TierNewUser            = "new_user"
	TierContributor        = "contributor"
	TierTrustedContributor = "trusted_contributor"
	TierLocalAmbassador    = "local_ambassador"
)

// Promotion thresholds.
const (
	ContributorMinEdits           = 5
	ContributorMinAccountAge      = 14 * 24 * time.Hour // 2 weeks
	TrustedMinEdits               = 25
	TrustedMinApprovalRate        = 0.95
	TrustedMinAccountAge          = 60 * 24 * time.Hour // ~2 months
	AmbassadorMinEdits            = 50
	AmbassadorMinAccountAge       = 180 * 24 * time.Hour // ~6 months
	AmbassadorMinCityEdits        = 10
	DemotionApprovalRateThreshold = 0.80
	DemotionMinEditsForRate       = 3 // must have at least 3 edits in 30d window to evaluate rate
)

// DefaultAutoPromotionInterval is the default interval for the background scheduler (24 hours).
const DefaultAutoPromotionInterval = 24 * time.Hour

// tierOrder maps tiers to their ordinal position (for promotion/demotion logic).
var tierOrder = map[string]int{
	TierNewUser:            0,
	TierContributor:        1,
	TierTrustedContributor: 2,
	TierLocalAmbassador:    3,
}

// tierByOrder maps ordinal position back to tier name.
var tierByOrder = map[int]string{
	0: TierNewUser,
	1: TierContributor,
	2: TierTrustedContributor,
	3: TierLocalAmbassador,
}

// AutoPromotionService evaluates users for tier promotion and demotion.
type AutoPromotionService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
}

// NewAutoPromotionService creates a new auto-promotion service.
func NewAutoPromotionService(database *gorm.DB, emailService contracts.EmailServiceInterface) *AutoPromotionService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultAutoPromotionInterval
	if envInterval := os.Getenv("AUTO_PROMOTION_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	return &AutoPromotionService{
		db:           database,
		emailService: emailService,
		interval:     interval,
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
}

// Start begins the background auto-promotion scheduler.
func (s *AutoPromotionService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("auto-promotion scheduler started",
		"interval_hours", s.interval.Hours(),
	)
}

// Stop gracefully stops the auto-promotion scheduler.
func (s *AutoPromotionService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("auto-promotion scheduler stopped")
}

// run is the main loop for the auto-promotion scheduler.
// Panic recovery via shared.RunTickerLoop (PSY-615).
func (s *AutoPromotionService) run(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "auto_promotion", s.interval, s.stopCh, true, func(_ context.Context) {
		s.runEvaluationCycle()
	})
}

// runEvaluationCycle performs a single evaluation of all users.
func (s *AutoPromotionService) runEvaluationCycle() {
	s.logger.Info("starting auto-promotion evaluation cycle")

	result, err := s.EvaluateAllUsers()
	if err != nil {
		s.logger.Error("auto-promotion evaluation failed", "error", err)
		return
	}

	s.logger.Info("auto-promotion evaluation cycle completed",
		"promoted", len(result.Promoted),
		"demoted", len(result.Demoted),
		"unchanged", result.Unchanged,
		"errors", result.Errors,
	)

	for _, change := range result.Promoted {
		s.logger.Info("user promoted",
			"user_id", change.UserID,
			"username", change.Username,
			"old_tier", change.OldTier,
			"new_tier", change.NewTier,
			"reason", change.Reason,
		)
	}
	for _, change := range result.Demoted {
		s.logger.Info("user demoted",
			"user_id", change.UserID,
			"username", change.Username,
			"old_tier", change.OldTier,
			"new_tier", change.NewTier,
			"reason", change.Reason,
		)
	}
}

// RunEvaluationNow triggers an immediate evaluation cycle (useful for testing).
func (s *AutoPromotionService) RunEvaluationNow() {
	s.runEvaluationCycle()
}

// EvaluateAllUsers checks all active, non-admin users for promotion/demotion eligibility.
func (s *AutoPromotionService) EvaluateAllUsers() (*contracts.AutoPromotionResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var users []authm.User
	err := s.db.Where("is_active = ? AND is_admin = ? AND deleted_at IS NULL", true, false).
		Find(&users).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}

	result := &contracts.AutoPromotionResult{
		Promoted: []contracts.UserTierChange{},
		Demoted:  []contracts.UserTierChange{},
	}

	for _, user := range users {
		evalResult, err := s.evaluateUserInternal(&user)
		if err != nil {
			s.logger.Error("failed to evaluate user",
				"user_id", user.ID,
				"error", err,
			)
			result.Errors++
			continue
		}

		if !evalResult.Changed {
			result.Unchanged++
			continue
		}

		username := ""
		if user.Username != nil {
			username = *user.Username
		}

		change := contracts.UserTierChange{
			UserID:   user.ID,
			Username: username,
			OldTier:  evalResult.CurrentTier,
			NewTier:  evalResult.NewTier,
			Reason:   evalResult.Reason,
		}

		// Apply the tier change
		if err := s.db.Model(&authm.User{}).Where("id = ?", user.ID).
			Update("user_tier", evalResult.NewTier).Error; err != nil {
			s.logger.Error("failed to update user tier",
				"user_id", user.ID,
				"error", err,
			)
			result.Errors++
			continue
		}

		isPromotion := tierOrder[evalResult.NewTier] > tierOrder[evalResult.CurrentTier]
		if isPromotion {
			result.Promoted = append(result.Promoted, change)
		} else {
			result.Demoted = append(result.Demoted, change)
		}

		// Fire-and-forget: send email notification
		s.sendTierChangeEmail(&user, change, isPromotion)

		// Fire-and-forget: write audit log
		s.writeTierChangeAuditLog(&user, change, isPromotion)
	}

	return result, nil
}

// sendTierChangeEmail sends a promotion or demotion email for a tier change.
// Errors are logged but never fail the parent operation.
func (s *AutoPromotionService) sendTierChangeEmail(user *authm.User, change contracts.UserTierChange, isPromotion bool) {
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return
	}

	if user.Email == nil || *user.Email == "" {
		return
	}

	email := *user.Email
	username := change.Username

	if isPromotion {
		newPermissions := notification.TierPermissions(change.NewTier)
		if err := s.emailService.SendTierPromotionEmail(email, username, change.OldTier, change.NewTier, change.Reason, newPermissions); err != nil {
			s.logger.Error("failed to send tier promotion email",
				"user_id", user.ID,
				"error", err,
			)
		}
	} else {
		if err := s.emailService.SendTierDemotionEmail(email, username, change.OldTier, change.NewTier, change.Reason); err != nil {
			s.logger.Error("failed to send tier demotion email",
				"user_id", user.ID,
				"error", err,
			)
		}
	}
}

// writeTierChangeAuditLog records a tier change in the audit log.
// Errors are logged but never fail the parent operation.
func (s *AutoPromotionService) writeTierChangeAuditLog(user *authm.User, change contracts.UserTierChange, isPromotion bool) {
	if s.db == nil {
		return
	}

	action := "tier_promotion"
	if !isPromotion {
		action = "tier_demotion"
	}

	metadata := map[string]interface{}{
		"old_tier": change.OldTier,
		"new_tier": change.NewTier,
		"reason":   change.Reason,
		"username": change.Username,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Error("failed to marshal audit log metadata",
			"user_id", user.ID,
			"error", err,
		)
		return
	}

	raw := json.RawMessage(metadataJSON)
	auditLog := adminm.AuditLog{
		ActorID:    nil, // system action, no actor
		Action:     action,
		EntityType: "user",
		EntityID:   user.ID,
		Metadata:   &raw,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&auditLog).Error; err != nil {
		s.logger.Error("failed to write tier change audit log",
			"user_id", user.ID,
			"error", err,
		)
	}
}

// EvaluateUser checks a single user for promotion/demotion eligibility.
func (s *AutoPromotionService) EvaluateUser(userID uint) (*contracts.UserEvaluationResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user authm.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return s.evaluateUserInternal(&user)
}

// evaluateUserInternal computes promotion/demotion for a single user.
func (s *AutoPromotionService) evaluateUserInternal(user *authm.User) (*contracts.UserEvaluationResult, error) {
	now := time.Now()
	accountAge := now.Sub(user.CreatedAt)

	// Count approved edits from pending_entity_edits
	var pendingApproved int64
	if err := s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND status = ?", user.ID, adminm.PendingEditStatusApproved).
		Count(&pendingApproved).Error; err != nil {
		return nil, fmt.Errorf("failed to count approved pending edits: %w", err)
	}

	// Count total edits from pending_entity_edits (approved + rejected)
	var pendingTotal int64
	if err := s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ?", user.ID).
		Count(&pendingTotal).Error; err != nil {
		return nil, fmt.Errorf("failed to count total pending edits: %w", err)
	}

	// Count revisions (direct edits by trusted users create revisions, not pending edits)
	var revisionCount int64
	if err := s.db.Model(&adminm.Revision{}).
		Where("user_id = ?", user.ID).
		Count(&revisionCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count revisions: %w", err)
	}

	totalApproved := int(pendingApproved) + int(revisionCount)
	totalEdits := int(pendingTotal) + int(revisionCount)

	// Calculate approval rate (revisions are always "approved" — they're direct edits)
	var approvalRate float64
	if totalEdits > 0 {
		approvalRate = float64(totalApproved) / float64(totalEdits)
	}

	// Rolling 30-day stats for demotion check
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)

	var rolling30dApproved int64
	if err := s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND status = ? AND created_at >= ?", user.ID, adminm.PendingEditStatusApproved, thirtyDaysAgo).
		Count(&rolling30dApproved).Error; err != nil {
		return nil, fmt.Errorf("failed to count rolling 30d approved: %w", err)
	}

	var rolling30dTotal int64
	if err := s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND created_at >= ?", user.ID, thirtyDaysAgo).
		Count(&rolling30dTotal).Error; err != nil {
		return nil, fmt.Errorf("failed to count rolling 30d total: %w", err)
	}

	var rolling30dRevisions int64
	if err := s.db.Model(&adminm.Revision{}).
		Where("user_id = ? AND created_at >= ?", user.ID, thirtyDaysAgo).
		Count(&rolling30dRevisions).Error; err != nil {
		return nil, fmt.Errorf("failed to count rolling 30d revisions: %w", err)
	}

	rolling30dApprovedTotal := int(rolling30dApproved) + int(rolling30dRevisions)
	rolling30dEditTotal := int(rolling30dTotal) + int(rolling30dRevisions)

	var rolling30dRate float64
	if rolling30dEditTotal > 0 {
		rolling30dRate = float64(rolling30dApprovedTotal) / float64(rolling30dEditTotal)
	}

	// Count city edits for local ambassador check
	cityEditCount := s.countCityEdits(user.ID)

	eval := &contracts.UserEvaluationResult{
		UserID:          user.ID,
		CurrentTier:     user.UserTier,
		ApprovedEdits:   totalApproved,
		TotalEdits:      totalEdits,
		ApprovalRate:    approvalRate,
		AccountAge:      accountAge,
		EmailVerified:   user.EmailVerified,
		CityEditCount:   cityEditCount,
		Rolling30dRate:  rolling30dRate,
		Rolling30dTotal: rolling30dEditTotal,
	}

	// Check demotion first (rolling 30-day window)
	if s.shouldDemote(user, rolling30dRate, rolling30dEditTotal) {
		currentOrder := tierOrder[user.UserTier]
		if currentOrder > 0 {
			newTier := tierByOrder[currentOrder-1]
			eval.Changed = true
			eval.NewTier = newTier
			eval.Reason = fmt.Sprintf("approval rate %.0f%% below 80%% threshold in rolling 30-day window (%d edits)", rolling30dRate*100, rolling30dEditTotal)
			return eval, nil
		}
	}

	// Check promotion (at most one tier per evaluation)
	if promoted, newTier, reason := s.shouldPromote(user, totalApproved, approvalRate, accountAge, cityEditCount); promoted {
		eval.Changed = true
		eval.NewTier = newTier
		eval.Reason = reason
		return eval, nil
	}

	return eval, nil
}

// shouldDemote checks if the user should be demoted based on rolling 30-day approval rate.
func (s *AutoPromotionService) shouldDemote(user *authm.User, rolling30dRate float64, rolling30dTotal int) bool {
	// Can't demote below new_user
	if user.UserTier == TierNewUser {
		return false
	}
	// Need minimum edits in the window to evaluate rate
	if rolling30dTotal < DemotionMinEditsForRate {
		return false
	}
	return rolling30dRate < DemotionApprovalRateThreshold
}

// shouldPromote checks if the user should be promoted and returns (shouldPromote, newTier, reason).
func (s *AutoPromotionService) shouldPromote(user *authm.User, approvedEdits int, approvalRate float64, accountAge time.Duration, cityEdits int) (bool, string, string) {
	switch user.UserTier {
	case TierNewUser:
		if approvedEdits >= ContributorMinEdits &&
			accountAge >= ContributorMinAccountAge &&
			user.EmailVerified {
			return true, TierContributor, fmt.Sprintf(
				"%d approved edits, account age %d days, email verified",
				approvedEdits, int(accountAge.Hours()/24),
			)
		}
	case TierContributor:
		if approvedEdits >= TrustedMinEdits &&
			approvalRate >= TrustedMinApprovalRate &&
			accountAge >= TrustedMinAccountAge {
			return true, TierTrustedContributor, fmt.Sprintf(
				"%d approved edits, %.0f%% approval rate, account age %d days",
				approvedEdits, approvalRate*100, int(accountAge.Hours()/24),
			)
		}
	case TierTrustedContributor:
		if approvedEdits >= AmbassadorMinEdits &&
			accountAge >= AmbassadorMinAccountAge &&
			cityEdits >= AmbassadorMinCityEdits {
			return true, TierLocalAmbassador, fmt.Sprintf(
				"%d approved edits, %d city edits, account age %d days",
				approvedEdits, cityEdits, int(accountAge.Hours()/24),
			)
		}
	case TierLocalAmbassador:
		// Already at the highest tier
	}
	return false, "", ""
}

// countCityEdits counts edits related to a specific city for a user.
// It looks at pending edits on venues (which have a city) and revisions on venues.
func (s *AutoPromotionService) countCityEdits(userID uint) int {
	// Count pending edits on venues (venues always have a city)
	var venueEdits int64
	s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND entity_type = ? AND status = ?", userID, "venue", adminm.PendingEditStatusApproved).
		Count(&venueEdits)

	// Count pending edits on artists (artists may have a city)
	var artistEdits int64
	s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND entity_type = ? AND status = ?", userID, "artist", adminm.PendingEditStatusApproved).
		Count(&artistEdits)

	// Count revisions on venues
	var venueRevisions int64
	s.db.Model(&adminm.Revision{}).
		Where("user_id = ? AND entity_type = ?", userID, "venue").
		Count(&venueRevisions)

	// Count revisions on artists
	var artistRevisions int64
	s.db.Model(&adminm.Revision{}).
		Where("user_id = ? AND entity_type = ?", userID, "artist").
		Count(&artistRevisions)

	return int(venueEdits + artistEdits + venueRevisions + artistRevisions)
}
