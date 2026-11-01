package community

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The sections, activity-heatmap and rankings handlers resolve their target
// through resolveProfileTarget: a failed lookup is a 500, the owner of a
// private profile reads it, and the granular contributions setting still
// shapes the rankings answer for everyone else.

func privacyRaw(t *testing.T, contributions contracts.PrivacyLevel) *json.RawMessage {
	t.Helper()
	settings := contracts.DefaultPrivacySettings()
	settings.Contributions = contributions
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal privacy settings: %v", err)
	}
	msg := json.RawMessage(raw)
	return &msg
}

func failingUserLookup() *testhelpers.MockUserService {
	return &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return nil, fmt.Errorf("db error") },
	}
}

func TestGetUserSections_UserLookupErrorIs500(t *testing.T) {
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, failingUserLookup(), nil, nil)
	_, err := h.GetUserSectionsHandler(context.Background(), &GetUserSectionsRequest{Username: "someone"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetActivityHeatmap_UserLookupErrorIs500(t *testing.T) {
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, failingUserLookup(), nil, nil)
	_, err := h.GetActivityHeatmapHandler(context.Background(), &GetActivityHeatmapRequest{Username: "someone"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetPercentileRankings_OwnerReadsOwnPrivateProfile(t *testing.T) {
	owner := &authm.User{ID: 5, ProfileVisibility: authm.ProfileVisibilityPrivate, IsActive: true}
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return owner, nil },
	}
	profile := &testhelpers.MockContributorProfileService{
		GetPercentileRankingsFn: func(uint) (*contracts.PercentileRankings, error) {
			return &contracts.PercentileRankings{Rankings: []contracts.PercentileRanking{{Dimension: "shows_submitted"}}, OverallScore: 40}, nil
		},
	}
	h := NewContributorProfileHandler(profile, users, nil, nil)

	resp, err := h.GetPercentileRankingsHandler(testhelpers.CtxWithUser(&authm.User{ID: 5}), &GetPercentileRankingsRequest{Username: "owner"})
	if err != nil {
		t.Fatalf("owner read of a private profile's rankings: %v", err)
	}
	if len(resp.Body.Rankings) != 1 || resp.Body.OverallScore != 40 {
		t.Errorf("owner got %+v, want the full rankings", resp.Body)
	}
}

func TestGetPercentileRankings_ContributionsHiddenIs404ForOthers(t *testing.T) {
	target := &authm.User{ID: 5, ProfileVisibility: authm.ProfileVisibilityPublic, IsActive: true, PrivacySettings: privacyRaw(t, contracts.PrivacyHidden)}
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return target, nil },
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, users, nil, nil)

	_, err := h.GetPercentileRankingsHandler(testhelpers.CtxWithUser(&authm.User{ID: 6}), &GetPercentileRankingsRequest{Username: "target"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetPercentileRankings_ContributionsCountOnlyIsScoreOnlyForOthers(t *testing.T) {
	target := &authm.User{ID: 5, ProfileVisibility: authm.ProfileVisibilityPublic, IsActive: true, PrivacySettings: privacyRaw(t, contracts.PrivacyCountOnly)}
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return target, nil },
	}
	profile := &testhelpers.MockContributorProfileService{
		GetPercentileRankingsFn: func(uint) (*contracts.PercentileRankings, error) {
			return &contracts.PercentileRankings{Rankings: []contracts.PercentileRanking{{Dimension: "shows_submitted"}}, OverallScore: 40}, nil
		},
	}
	h := NewContributorProfileHandler(profile, users, nil, nil)

	resp, err := h.GetPercentileRankingsHandler(testhelpers.CtxWithUser(&authm.User{ID: 6}), &GetPercentileRankingsRequest{Username: "target"})
	if err != nil {
		t.Fatalf("count_only rankings: %v", err)
	}
	if len(resp.Body.Rankings) != 0 || resp.Body.OverallScore != 40 {
		t.Errorf("count_only rankings = %+v, want the overall score alone", resp.Body)
	}
}
