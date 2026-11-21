package shared

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
)

// The far-future refusal reaches the caller with its own message even when a
// generic create failure wraps it, which is how the request fulfiller returns
// it; the generic message would hide the bound and the override.
func TestMapShowError_SurfacesTheWrappedDateBoundRefusal(t *testing.T) {
	bound := time.Date(2028, 5, 20, 18, 0, 0, 0, time.UTC)
	refusal := apperrors.ErrShowEventDateBeyondBound(bound.AddDate(0, 1, 0), bound, 18)

	for name, err := range map[string]error{
		"bare":                          refusal,
		"inside a create failure":       apperrors.ErrShowCreateFailed(refusal),
		"inside fmt and create failure": fmt.Errorf("fulfil: %w", apperrors.ErrShowCreateFailed(fmt.Errorf("create: %w", refusal))),
	} {
		t.Run(name, func(t *testing.T) {
			mapped := MapShowError(err)
			if mapped == nil {
				t.Fatal("not mapped")
			}
			if got := statusOf(t, mapped); got != http.StatusUnprocessableEntity {
				t.Fatalf("status %d, want 422", got)
			}
			if !strings.Contains(mapped.Error(), "Shows can be listed up to 18 months ahead. If this date is right, contact us and we'll add it.") {
				t.Fatalf("message lost: %v", mapped)
			}
		})
	}

	// A create failure that wraps something else keeps the generic message.
	generic := MapShowError(apperrors.ErrShowCreateFailed(fmt.Errorf("db down")))
	if generic == nil || strings.Contains(generic.Error(), "months ahead") {
		t.Fatalf("generic failure mapped as %v", generic)
	}
}
