package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

type mockEntityExistenceService struct {
	exists bool
	err    error
}

func (m mockEntityExistenceService) Exists(entityType, idOrSlug string) (bool, error) {
	return m.exists, m.err
}

func TestEntityExistsHandler_Success(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: true})

	resp, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "shows",
		EntityID:   "approved-show",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestEntityExistsHandler_NotFound(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{exists: false})

	_, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "tags",
		EntityID:   "missing-tag",
	})

	testhelpers.AssertHumaError(t, err, 404)
}

func TestEntityExistsHandler_ServiceError(t *testing.T) {
	h := NewEntityExistenceHandler(mockEntityExistenceService{err: fmt.Errorf("database down")})

	_, err := h.EntityExistsHandler(context.Background(), &EntityExistsRequest{
		EntityType: "scenes",
		EntityID:   "phoenix-az",
	})

	testhelpers.AssertHumaError(t, err, 500)
}
