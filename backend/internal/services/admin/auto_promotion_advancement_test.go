package admin

import (
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

func TestBuildAdvancementProgress_NewUser(t *testing.T) {
	eval := &contracts.UserEvaluationResult{
		UserID:        1,
		CurrentTier:   TierNewUser,
		ApprovedEdits: 3,
		AccountAge:    10 * 24 * time.Hour,
		EmailVerified: true,
	}
	got := buildAdvancementProgress(eval)
	if got.CurrentTier != TierNewUser || got.NextTier != TierContributor {
		t.Fatalf("tiers: current=%q next=%q", got.CurrentTier, got.NextTier)
	}
	if len(got.Requirements) != 3 {
		t.Fatalf("want 3 requirements, got %d", len(got.Requirements))
	}
	assertNumericReq(t, got.Requirements[0], contracts.AdvancementReqApprovedEdits, 3, 5, false)
	assertNumericReq(t, got.Requirements[1], contracts.AdvancementReqAccountAgeDays, 10, 14, false)
	assertBoolReq(t, got.Requirements[2], contracts.AdvancementReqEmailVerified, true)
}

func TestBuildAdvancementProgress_NewUserFullyMet(t *testing.T) {
	eval := &contracts.UserEvaluationResult{
		CurrentTier:   TierNewUser,
		ApprovedEdits: 5,
		AccountAge:    14 * 24 * time.Hour,
		EmailVerified: true,
	}
	got := buildAdvancementProgress(eval)
	for _, req := range got.Requirements {
		if !req.Met {
			t.Errorf("expected %s met", req.Requirement)
		}
	}
}

func TestBuildAdvancementProgress_Contributor(t *testing.T) {
	eval := &contracts.UserEvaluationResult{
		CurrentTier:   TierContributor,
		ApprovedEdits: 20,
		ApprovalRate:  0.97,
		AccountAge:    45 * 24 * time.Hour,
	}
	got := buildAdvancementProgress(eval)
	if got.NextTier != TierTrustedContributor {
		t.Fatalf("next=%q", got.NextTier)
	}
	if len(got.Requirements) != 3 {
		t.Fatalf("want 3 requirements, got %d", len(got.Requirements))
	}
	assertNumericReq(t, got.Requirements[0], contracts.AdvancementReqApprovedEdits, 20, 25, false)
	assertNumericReq(t, got.Requirements[1], contracts.AdvancementReqApprovalRate, 97, 95, true)
	assertNumericReq(t, got.Requirements[2], contracts.AdvancementReqAccountAgeDays, 45, 60, false)
}

func TestBuildAdvancementProgress_TrustedContributor(t *testing.T) {
	eval := &contracts.UserEvaluationResult{
		CurrentTier:   TierTrustedContributor,
		ApprovedEdits: 32,
		CityEditCount: 8,
		AccountAge:    200 * 24 * time.Hour,
	}
	got := buildAdvancementProgress(eval)
	if got.NextTier != TierLocalAmbassador {
		t.Fatalf("next=%q", got.NextTier)
	}
	assertNumericReq(t, got.Requirements[0], contracts.AdvancementReqApprovedEdits, 32, 50, false)
	assertNumericReq(t, got.Requirements[1], contracts.AdvancementReqCityEdits, 8, 10, false)
	assertNumericReq(t, got.Requirements[2], contracts.AdvancementReqAccountAgeDays, 200, 180, true)
}

func TestBuildAdvancementProgress_LocalAmbassador(t *testing.T) {
	eval := &contracts.UserEvaluationResult{
		CurrentTier:   TierLocalAmbassador,
		ApprovedEdits: 100,
	}
	got := buildAdvancementProgress(eval)
	if got.NextTier != "" {
		t.Errorf("highest tier should have empty next_tier, got %q", got.NextTier)
	}
	if len(got.Requirements) != 0 {
		t.Errorf("highest tier should have no requirements, got %d", len(got.Requirements))
	}
}

func TestBuildAdvancementProgress_OmitsDemotionFields(t *testing.T) {
	// AdvancementProgress has no Rolling30d* fields by construction; this
	// locks the DTO shape so a future edit can't accidentally re-expose them
	// via embedding UserEvaluationResult.
	eval := &contracts.UserEvaluationResult{
		CurrentTier:     TierContributor,
		ApprovedEdits:   10,
		ApprovalRate:    0.5,
		AccountAge:      30 * 24 * time.Hour,
		Rolling30dRate:  0.1,
		Rolling30dTotal: 20,
	}
	got := buildAdvancementProgress(eval)
	if got.CurrentTier != TierContributor {
		t.Fatal("unexpected current tier")
	}
	// No demotion requirements should appear.
	for _, req := range got.Requirements {
		if req.Requirement == "rolling_30d_rate" || req.Requirement == "rolling_30d_total" {
			t.Errorf("demotion metric leaked: %s", req.Requirement)
		}
	}
}

func TestGetAdvancementProgress_NilDB(t *testing.T) {
	svc := &AutoPromotionService{}
	_, err := svc.GetAdvancementProgress(1)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func assertNumericReq(t *testing.T, req contracts.AdvancementRequirement, id string, current, threshold float64, met bool) {
	t.Helper()
	if req.Requirement != id {
		t.Errorf("requirement id: want %q got %q", id, req.Requirement)
	}
	if req.Current == nil || *req.Current != current {
		t.Errorf("%s current: want %v got %v", id, current, req.Current)
	}
	if req.Threshold == nil || *req.Threshold != threshold {
		t.Errorf("%s threshold: want %v got %v", id, threshold, req.Threshold)
	}
	if req.Met != met {
		t.Errorf("%s met: want %v got %v", id, met, req.Met)
	}
}

func assertBoolReq(t *testing.T, req contracts.AdvancementRequirement, id string, met bool) {
	t.Helper()
	if req.Requirement != id {
		t.Errorf("requirement id: want %q got %q", id, req.Requirement)
	}
	if req.Current != nil || req.Threshold != nil {
		t.Errorf("%s should omit current/threshold", id)
	}
	if req.Met != met {
		t.Errorf("%s met: want %v got %v", id, met, req.Met)
	}
}
