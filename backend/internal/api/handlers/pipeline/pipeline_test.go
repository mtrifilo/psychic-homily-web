package pipeline

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func pipelineAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

// ============================================================================
// Tests: EnrichmentStatusHandler
// ============================================================================

func TestPipelineHandler_EnrichmentStatus_Success(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockEnrichmentService{
			GetQueueStatsFn: func() (*contracts.EnrichmentQueueStats, error) {
				return &contracts.EnrichmentQueueStats{
					Pending:        5,
					Processing:     2,
					CompletedToday: 10,
					FailedToday:    1,
				}, nil
			},
		},
	)

	resp, err := h.EnrichmentStatusHandler(pipelineAdminCtx(), &EnrichmentStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Pending != 5 {
		t.Errorf("expected pending=5, got %d", resp.Body.Pending)
	}
	if resp.Body.Processing != 2 {
		t.Errorf("expected processing=2, got %d", resp.Body.Processing)
	}
	if resp.Body.CompletedToday != 10 {
		t.Errorf("expected completed_today=10, got %d", resp.Body.CompletedToday)
	}
	if resp.Body.FailedToday != 1 {
		t.Errorf("expected failed_today=1, got %d", resp.Body.FailedToday)
	}
}

func TestPipelineHandler_EnrichmentStatus_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockEnrichmentService{
			GetQueueStatsFn: func() (*contracts.EnrichmentQueueStats, error) {
				return nil, fmt.Errorf("database error")
			},
		},
	)

	_, err := h.EnrichmentStatusHandler(pipelineAdminCtx(), &EnrichmentStatusRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: TriggerEnrichmentHandler
// ============================================================================

func TestPipelineHandler_TriggerEnrichment_Success(t *testing.T) {
	var receivedShowID uint
	var receivedType string
	h := NewPipelineHandler(
		&testhelpers.MockEnrichmentService{
			QueueShowForEnrichmentFn: func(showID uint, enrichmentType string) error {
				receivedShowID = showID
				receivedType = enrichmentType
				return nil
			},
		},
	)

	resp, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if receivedShowID != 42 {
		t.Errorf("expected showID=42, got %d", receivedShowID)
	}
	if receivedType != "all" {
		t.Errorf("expected type=all, got %s", receivedType)
	}
}

func TestPipelineHandler_TriggerEnrichment_InvalidShowID(t *testing.T) {
	h := NewPipelineHandler(&testhelpers.MockEnrichmentService{})

	_, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestPipelineHandler_TriggerEnrichment_ServiceError(t *testing.T) {
	h := NewPipelineHandler(
		&testhelpers.MockEnrichmentService{
			QueueShowForEnrichmentFn: func(showID uint, enrichmentType string) error {
				return fmt.Errorf("show not found")
			},
		},
	)

	_, err := h.TriggerEnrichmentHandler(pipelineAdminCtx(), &TriggerEnrichmentRequest{ShowID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}
