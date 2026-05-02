package pipeline

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// adminCtx returns a context populated with an admin user, suitable for
// invoking handlers that gate on shared.RequireAdmin.
func adminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func testAdminDiscoveryHandler() *AdminDiscoveryHandler {
	return NewAdminDiscoveryHandler(nil)
}

func adminDiscoveryHandler(opts ...func(*AdminDiscoveryHandler)) *AdminDiscoveryHandler {
	h := &AdminDiscoveryHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// DiscoveryImportHandler — empty events
func TestDiscoveryImportHandler_EmptyEvents(t *testing.T) {
	h := testAdminDiscoveryHandler()
	req := &DiscoveryImportRequest{}

	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// DiscoveryImportHandler — too many events
func TestDiscoveryImportHandler_TooMany(t *testing.T) {
	h := testAdminDiscoveryHandler()
	req := &DiscoveryImportRequest{}
	req.Body.Events = make([]DiscoveryImportEventInput, 101)

	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// DiscoveryCheckHandler — empty events
func TestDiscoveryCheckHandler_EmptyEvents(t *testing.T) {
	h := testAdminDiscoveryHandler()
	req := &DiscoveryCheckRequest{}

	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// DiscoveryCheckHandler — too many events
func TestDiscoveryCheckHandler_TooMany(t *testing.T) {
	h := testAdminDiscoveryHandler()
	req := &DiscoveryCheckRequest{}
	req.Body.Events = make([]DiscoveryCheckEventInput, 201)

	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestDiscoveryImportHandler_Success(t *testing.T) {
	h := adminDiscoveryHandler(func(ah *AdminDiscoveryHandler) {
		ah.discoveryService = &testhelpers.MockDiscoveryService{
			ImportEventsFn: func(events []contracts.DiscoveredEvent, dryRun, allowUpdates bool, initialStatus catalogm.ShowStatus) (*contracts.ImportResult, error) {
				return &contracts.ImportResult{Total: len(events), Imported: len(events)}, nil
			},
		}
	})
	req := &DiscoveryImportRequest{}
	req.Body.Events = []DiscoveryImportEventInput{{ID: "ev1", VenueSlug: "test"}}
	resp, err := h.DiscoveryImportHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Imported != 1 {
		t.Errorf("expected imported=1, got %d", resp.Body.Imported)
	}
}

func TestDiscoveryImportHandler_ServiceError(t *testing.T) {
	h := adminDiscoveryHandler(func(ah *AdminDiscoveryHandler) {
		ah.discoveryService = &testhelpers.MockDiscoveryService{
			ImportEventsFn: func(_ []contracts.DiscoveredEvent, _, _ bool, _ catalogm.ShowStatus) (*contracts.ImportResult, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &DiscoveryImportRequest{}
	req.Body.Events = []DiscoveryImportEventInput{{ID: "ev1"}}
	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestDiscoveryCheckHandler_Success(t *testing.T) {
	h := adminDiscoveryHandler(func(ah *AdminDiscoveryHandler) {
		ah.discoveryService = &testhelpers.MockDiscoveryService{
			CheckEventsFn: func(events []contracts.CheckEventInput) (*contracts.CheckEventsResult, error) {
				return &contracts.CheckEventsResult{}, nil
			},
		}
	})
	req := &DiscoveryCheckRequest{}
	req.Body.Events = []DiscoveryCheckEventInput{{ID: "ev1", VenueSlug: "test"}}
	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryCheckHandler_ServiceError(t *testing.T) {
	h := adminDiscoveryHandler(func(ah *AdminDiscoveryHandler) {
		ah.discoveryService = &testhelpers.MockDiscoveryService{
			CheckEventsFn: func(_ []contracts.CheckEventInput) (*contracts.CheckEventsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &DiscoveryCheckRequest{}
	req.Body.Events = []DiscoveryCheckEventInput{{ID: "ev1"}}
	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}
