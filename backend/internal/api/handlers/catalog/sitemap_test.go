package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

type stubSitemapService struct {
	entries *contracts.SitemapEntries
	err     error
}

func (s stubSitemapService) Entries(context.Context) (*contracts.SitemapEntries, error) {
	return s.entries, s.err
}

// TestSitemapEntriesHandlerFailsClosed is the handler-level half of the
// fail-closed contract. The generator only survives a backend fault because a
// fault surfaces AS a fault — a 200 with an empty body here would reproduce the
// original incident one layer down, and look perfectly healthy doing it.
func TestSitemapEntriesHandlerFailsClosed(t *testing.T) {
	handler := NewSitemapHandler(stubSitemapService{
		err: errors.New(`ERROR: column "slug" does not exist (SQLSTATE 42703)`),
	})

	resp, err := handler.GetSitemapEntriesHandler(context.Background(), &GetSitemapEntriesRequest{})

	if err == nil {
		t.Fatalf("expected an error, got a %+v response — a service failure must not render as success", resp)
	}

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if statusErr.GetStatus() != 500 {
		t.Errorf("status = %d, want 500", statusErr.GetStatus())
	}

	// The raw driver text must not reach an unauthenticated caller: it is
	// already logged with the request ID, and echoing it discloses schema and
	// SQLSTATE for free on a public endpoint.
	if strings.Contains(err.Error(), "SQLSTATE") || strings.Contains(err.Error(), "column") {
		t.Errorf("error surfaced to the caller leaks the underlying DB error: %q", err.Error())
	}
}

// TestSitemapEntriesHandlerSetsCacheControl pins the shared-cache hint. The body
// is identical for every caller and the only consumer refetches hourly, so
// without this every crawler hit becomes three unbounded projections.
func TestSitemapEntriesHandlerSetsCacheControl(t *testing.T) {
	handler := NewSitemapHandler(stubSitemapService{
		entries: &contracts.SitemapEntries{},
	})

	resp, err := handler.GetSitemapEntriesHandler(context.Background(), &GetSitemapEntriesRequest{})
	if err != nil {
		t.Fatalf("GetSitemapEntriesHandler: %v", err)
	}
	if !strings.Contains(resp.CacheControl, "max-age=") {
		t.Errorf("Cache-Control = %q, want a max-age directive", resp.CacheControl)
	}
}
