package catalog

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

type stubSitemapService struct {
	entries *contracts.SitemapEntries
	err     error
}

func (s stubSitemapService) Entries(context.Context, string) (*contracts.SitemapEntries, error) {
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

// TestSitemapEntriesHandlerNilResultFailsClosed covers the (nil, nil) return.
// It cannot happen with *SitemapService, but this handler depends on the
// INTERFACE, and the zero-valued stub one file over already has that shape — so
// without this the branch reads as dead code and a deref panic is one careless
// simplification away.
func TestSitemapEntriesHandlerNilResultFailsClosed(t *testing.T) {
	handler := NewSitemapHandler(stubSitemapService{entries: nil, err: nil})

	resp, err := handler.GetSitemapEntriesHandler(context.Background(), &GetSitemapEntriesRequest{})

	if err == nil {
		t.Fatalf("expected an error, got a %+v response — a nil result must not render as success", resp)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) || statusErr.GetStatus() != 500 {
		t.Errorf("expected a 500 huma.StatusError, got %T %v", err, err)
	}
}

// TestSitemapEntriesHandlerSetsCacheControl pins the shared-cache hint. Crawlers
// do not reach this endpoint — they fetch /sitemap.xml from the frontend — so
// this bounds repeat hits from everything that is NOT the generator; see the
// note on sitemapEntriesCacheControl.
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

// TestSitemapFamilyEnumMatchesTheService keeps huma's request validation and
// the service's own guard from drifting apart.
//
// The enum is a struct tag, so it is a hand-written literal that no amount of
// care in the service can keep current. Drift is silent in the direction that
// matters most: a family or a sub-shard the service serves but the enum omits
// is rejected with a 422 BEFORE the handler runs, which the sitemap generator
// reads as "the backend does not serve this" and degrades to an empty
// document — thousands of URLs quietly leaving the index with a green build.
func TestSitemapFamilyEnumMatchesTheService(t *testing.T) {
	field, ok := reflect.TypeOf(GetSitemapEntriesRequest{}).FieldByName("Family")
	if !ok {
		t.Fatal("GetSitemapEntriesRequest has no Family field")
	}
	enum := field.Tag.Get("enum")
	if enum == "" {
		t.Fatal(`Family has no enum tag — huma would accept any value`)
	}

	got := strings.Split(enum, ",")
	want := catalog.SitemapFamilyValues()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enum tag = %v,\n            want %v\n"+
			"(the enum must list every value catalog.SitemapService.Entries accepts, in the same order)",
			got, want)
	}
}

// TestSitemapEntriesHandlerUnknownFamilyIs400 pins the family filter's
// fail-soft path: an unknown name must not become a 500 that looks like a DB
// fault.
func TestSitemapEntriesHandlerUnknownFamilyIs400(t *testing.T) {
	handler := NewSitemapHandler(stubSitemapService{
		err: errors.New(`unknown sitemap family "collections"`),
	})

	_, err := handler.GetSitemapEntriesHandler(context.Background(), &GetSitemapEntriesRequest{Family: "collections"})
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) || statusErr.GetStatus() != 400 {
		t.Errorf("expected a 400 huma.StatusError, got %T %v", err, err)
	}
}
