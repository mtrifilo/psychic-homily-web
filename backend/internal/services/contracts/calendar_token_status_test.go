package contracts

import (
	"encoding/json"
	"testing"
	"time"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

// WithoutFeedURLs keeps only whether a token exists and when it was minted:
// the observed reads and the link-reset marker are withheld along with the
// URLs, by construction, so none reaches the JSON an API-token caller gets.
func TestCalendarTokenStatusResponse_WithoutFeedURLsWithholdsReadsAndReset(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reason := engagementm.CalendarLinkResetPasswordReset
	full := CalendarTokenStatusResponse{
		HasToken:       true,
		CreatedAt:      &now,
		FeedURL:        "https://api.example.test/feeds/phcal_x/saved-shows.ics",
		FollowsFeedURL: "https://api.example.test/feeds/phcal_x/follows.atom",
		URLRecoverable: true,
		Reads: []CalendarFeedReadResponse{
			{Client: engagementm.CalendarFeedClientGoogle, FirstReadAt: now, LastReadAt: now},
		},
		LinkResetAt:     &now,
		LinkResetReason: &reason,
	}

	withheld := full.WithoutFeedURLs()

	if !withheld.HasToken || withheld.CreatedAt == nil || !withheld.CreatedAt.Equal(now) {
		t.Errorf("has_token and created_at must survive, got %+v", withheld)
	}
	raw, err := json.Marshal(withheld)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"reads", "link_reset_at", "link_reset_reason", "feed_url", "follows_feed_url"} {
		if _, present := fields[key]; present {
			t.Errorf("%s must be absent for a caller that may not receive the feed URLs, got %s", key, raw)
		}
	}
	if fields["url_recoverable"] != false {
		t.Errorf("url_recoverable = %v, want false", fields["url_recoverable"])
	}
}
