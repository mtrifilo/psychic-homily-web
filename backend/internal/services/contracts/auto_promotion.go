package contracts

import "time"

// ──────────────────────────────────────────────
// Auto-Promotion Service Interface
// ──────────────────────────────────────────────

// AutoPromotionServiceInterface defines the contract for evaluating user tier promotions/demotions.
type AutoPromotionServiceInterface interface {
	// EvaluateAllUsers checks all active, non-admin users for promotion/demotion eligibility.
	EvaluateAllUsers() (*AutoPromotionResult, error)

	// EvaluateUser checks a single user for promotion/demotion eligibility.
	EvaluateUser(userID uint) (*UserEvaluationResult, error)
}

// ──────────────────────────────────────────────
// Result Types
// ──────────────────────────────────────────────

// AutoPromotionResult contains the aggregate results of evaluating all users.
type AutoPromotionResult struct {
	Promoted  []UserTierChange `json:"promoted"`
	Demoted   []UserTierChange `json:"demoted"`
	Unchanged int              `json:"unchanged"`
	Errors    int              `json:"errors"`
}

// UserTierChange records a single user's tier change.
type UserTierChange struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	OldTier  string `json:"old_tier"`
	NewTier  string `json:"new_tier"`
	Reason   string `json:"reason"`
}

// UserEvaluationResult contains the detailed evaluation of a single user.
type UserEvaluationResult struct {
	UserID         uint          `json:"user_id"`
	CurrentTier    string        `json:"current_tier"`
	ApprovedEdits  int           `json:"approved_edits"`
	TotalEdits     int           `json:"total_edits"`
	ApprovalRate   float64       `json:"approval_rate"`
	AccountAge     time.Duration `json:"account_age"`
	EmailVerified  bool          `json:"email_verified"`
	CityEditCount  int           `json:"city_edit_count"`
	Changed        bool          `json:"changed"`
	NewTier        string        `json:"new_tier"`
	Reason         string        `json:"reason"`
	Rolling30dRate float64       `json:"rolling_30d_rate"`
	Rolling30dTotal int          `json:"rolling_30d_total"`
}
