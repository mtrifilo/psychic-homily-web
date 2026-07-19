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

	// GetAdvancementProgress returns the authenticated user's progress toward
	// the next tier. Self-scoped, read-only; omits demotion-watch metrics.
	GetAdvancementProgress(userID uint) (*AdvancementProgress, error)
}

// Advancement requirement ids (stable; mirrored by frontend/lib/tiers.ts).
const (
	AdvancementReqApprovedEdits  = "approved_edits"
	AdvancementReqAccountAgeDays = "account_age_days"
	AdvancementReqEmailVerified  = "email_verified"
	AdvancementReqApprovalRate   = "approval_rate"
	AdvancementReqCityEdits      = "city_edits"
)

// AdvancementProgress is the user-facing, self-scoped view of next-tier progress.
// Deliberately excludes demotion-watch fields (rolling 30d rate/total).
type AdvancementProgress struct {
	CurrentTier  string                   `json:"current_tier"`
	NextTier     string                   `json:"next_tier,omitempty"`
	Requirements []AdvancementRequirement `json:"requirements"`
}

// AdvancementRequirement is one gate on the path to the next tier.
// Numeric requirements include current + threshold; booleans omit both.
type AdvancementRequirement struct {
	Requirement string   `json:"requirement" doc:"Stable requirement id"`
	Current     *float64 `json:"current,omitempty" doc:"Current value (numeric requirements only)"`
	Threshold   *float64 `json:"threshold,omitempty" doc:"Required threshold (numeric requirements only)"`
	Met         bool     `json:"met" doc:"Whether this requirement is currently satisfied"`
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
	UserID          uint          `json:"user_id"`
	CurrentTier     string        `json:"current_tier"`
	ApprovedEdits   int           `json:"approved_edits"`
	TotalEdits      int           `json:"total_edits"`
	ApprovalRate    float64       `json:"approval_rate"`
	AccountAge      time.Duration `json:"account_age"`
	EmailVerified   bool          `json:"email_verified"`
	CityEditCount   int           `json:"city_edit_count"`
	Changed         bool          `json:"changed"`
	NewTier         string        `json:"new_tier"`
	Reason          string        `json:"reason"`
	Rolling30dRate  float64       `json:"rolling_30d_rate"`
	Rolling30dTotal int           `json:"rolling_30d_total"`
}
