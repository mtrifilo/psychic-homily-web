package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTierDisplayName(t *testing.T) {
	tests := []struct {
		tier     string
		expected string
	}{
		{"new_user", "New User"},
		{"contributor", "Contributor"},
		{"trusted_contributor", "Trusted Contributor"},
		{"local_ambassador", "Local Ambassador"},
		{"unknown_tier", "unknown_tier"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.tier, func(t *testing.T) {
			assert.Equal(t, tc.expected, TierDisplayName(tc.tier))
		})
	}
}

func TestTierPermissions(t *testing.T) {
	t.Run("new_user has no permissions", func(t *testing.T) {
		perms := TierPermissions("new_user")
		assert.Nil(t, perms)
	})

	t.Run("contributor has 3 permissions", func(t *testing.T) {
		perms := TierPermissions("contributor")
		assert.Len(t, perms, 3)
		assert.Contains(t, perms, "Submit edits for review")
		assert.Contains(t, perms, "Vote on tags and relationships")
		assert.Contains(t, perms, "Create collections")
	})

	t.Run("trusted_contributor has 2 permissions", func(t *testing.T) {
		perms := TierPermissions("trusted_contributor")
		assert.Len(t, perms, 2)
		assert.Contains(t, perms, "Edit entities directly (no review needed)")
		assert.Contains(t, perms, "Higher daily edit limit")
	})

	t.Run("local_ambassador has 2 permissions", func(t *testing.T) {
		perms := TierPermissions("local_ambassador")
		assert.Len(t, perms, 2)
		assert.Contains(t, perms, "All Trusted Contributor permissions")
		assert.Contains(t, perms, "Featured on city pages")
	})

	t.Run("unknown tier returns nil", func(t *testing.T) {
		perms := TierPermissions("unknown")
		assert.Nil(t, perms)
	})
}

func TestSendTierPromotionEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendTierPromotionEmail("test@example.com", "testuser", "new_user", "contributor", "5 approved edits", []string{"Submit edits"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email service is not configured")
}

func TestSendTierDemotionEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendTierDemotionEmail("test@example.com", "testuser", "contributor", "new_user", "low approval rate")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email service is not configured")
}

func TestSendTierDemotionWarningEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{}
	err := svc.SendTierDemotionWarningEmail("test@example.com", "testuser", "contributor", 0.82, 0.80)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email service is not configured")
}
