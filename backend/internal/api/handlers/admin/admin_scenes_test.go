package admin

import (
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// taglineHandler wires an AdminSceneHandler over a scene-service stub that
// records what the service layer was asked to write, plus an audit stub that
// records the entity-edit entry.
type taglineCapture struct {
	slug        string
	tagline     *string
	called      bool
	auditActor  uint
	auditEntity string
	auditID     uint
	auditMeta   map[string]interface{}
	audited     bool
}

func taglineHandler(t *testing.T, svcErr error) (*AdminSceneHandler, *taglineCapture) {
	t.Helper()
	rec := &taglineCapture{}
	scene := &testhelpers.MockSceneService{
		UpdateSceneTaglineFn: func(slug string, tagline *string) (uint, string, error) {
			rec.called = true
			rec.slug = slug
			rec.tagline = tagline
			if svcErr != nil {
				return 0, "", svcErr
			}
			return 42, "phoenix-az", nil
		},
	}
	audit := &testhelpers.MockAuditLogService{
		LogEntityEditFn: func(actorID uint, entityType string, entityID uint, meta map[string]interface{}) {
			rec.audited = true
			rec.auditActor = actorID
			rec.auditEntity = entityType
			rec.auditID = entityID
			rec.auditMeta = meta
		},
	}
	return NewAdminSceneHandler(scene, audit), rec
}

func taglineRequest(slug string, tagline *string) *UpdateSceneTaglineRequest {
	req := &UpdateSceneTaglineRequest{Slug: slug}
	req.Body.Tagline = tagline
	return req
}

func strptr(s string) *string { return &s }

func TestUpdateSceneTaglineHandler_SetsTagline(t *testing.T) {
	h, rec := taglineHandler(t, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	resp, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("phoenix-az", strptr("Where the desert learns to scream")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.tagline == nil || *rec.tagline != "Where the desert learns to scream" {
		t.Fatalf("service got tagline %v, want the authored line", rec.tagline)
	}
	if resp.Body.Tagline == nil || *resp.Body.Tagline != "Where the desert learns to scream" {
		t.Fatalf("response echoed %v, want the authored line", resp.Body.Tagline)
	}
	if resp.Body.Slug != "phoenix-az" {
		t.Fatalf("response slug = %q, want phoenix-az", resp.Body.Slug)
	}
}

// Authoring on a metro MEMBER city writes the metro's row, so the response
// (and the audit entry) must name the canonical scene rather than echoing the
// member slug back. Every scene read already answers with the canonical slug.
func TestUpdateSceneTaglineHandler_EchoesCanonicalSlug(t *testing.T) {
	h, rec := taglineHandler(t, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	resp, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("mesa-az", strptr("Desert noise")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The service still receives what the caller addressed; canonicalizing is
	// its job, not the handler's.
	if rec.slug != "mesa-az" {
		t.Fatalf("service got slug %q, want the caller's mesa-az", rec.slug)
	}
	if resp.Body.Slug != "phoenix-az" {
		t.Fatalf("response slug = %q, want the canonical phoenix-az", resp.Body.Slug)
	}
	if slug, _ := rec.auditMeta["slug"].(string); slug != "phoenix-az" {
		t.Fatalf("audit metadata slug = %v, want the canonical phoenix-az", rec.auditMeta["slug"])
	}
}

// Clearing must reach the service as a nil pointer — the service turns that
// into SQL NULL. Both spellings of "clear" (explicit null, empty string) and
// the whitespace-only case have to collapse to the same nil, or the page
// renders an invisible line that is neither present nor absent.
func TestUpdateSceneTaglineHandler_ClearingSpellings(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *string
	}{
		{"explicit null", nil},
		{"empty string", strptr("")},
		{"whitespace only", strptr("   \t  ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, rec := taglineHandler(t, nil)
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

			resp, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("phoenix-az", tc.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !rec.called {
				t.Fatal("service was not called")
			}
			if rec.tagline != nil {
				t.Fatalf("service got %q, want nil (clear)", *rec.tagline)
			}
			if resp.Body.Tagline != nil {
				t.Fatalf("response echoed %q, want null", *resp.Body.Tagline)
			}
			if cleared, _ := rec.auditMeta["cleared"].(bool); !cleared {
				t.Fatalf("audit metadata cleared = %v, want true", rec.auditMeta["cleared"])
			}
		})
	}
}

// Surrounding whitespace is trimmed rather than stored, so an authored line
// cannot smuggle leading padding past the length rec or into the markup.
func TestUpdateSceneTaglineHandler_TrimsSurroundingWhitespace(t *testing.T) {
	h, rec := taglineHandler(t, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	if _, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("phoenix-az", strptr("  Desert noise  "))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.tagline == nil || *rec.tagline != "Desert noise" {
		t.Fatalf("service got %v, want the trimmed line", rec.tagline)
	}
}

// The length guard is the boundary half of the VARCHAR(80) column. Counted in
// runes: a multi-byte tagline of 80 characters FITS the column, so a
// byte-length guard would reject a legal value.
func TestUpdateSceneTaglineHandler_LengthGuard(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tagline  string
		wantCall bool
	}{
		{"at the rec", strings.Repeat("a", MaxSceneTaglineLength), true},
		{"one over the rec", strings.Repeat("a", MaxSceneTaglineLength+1), false},
		{"multibyte at the rec", strings.Repeat("é", MaxSceneTaglineLength), true},
		{"multibyte over the rec", strings.Repeat("é", MaxSceneTaglineLength+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, rec := taglineHandler(t, nil)
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

			_, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("phoenix-az", &tc.tagline))
			if tc.wantCall {
				if err != nil {
					t.Fatalf("unexpected error for a tagline within the rec: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an over-length tagline to be rejected")
			}
			if !strings.Contains(err.Error(), "80 characters or fewer") {
				t.Fatalf("error = %q, want the length message", err.Error())
			}
			if rec.called {
				t.Fatal("over-length tagline reached the service; the guard must reject before the write")
			}
		})
	}
}

// An unresolvable slug is a 404, not a 500 — you cannot author a tagline for a
// city that is not a scene.
func TestUpdateSceneTaglineHandler_UnknownSceneIs404(t *testing.T) {
	h, _ := taglineHandler(t, apperrors.ErrSceneNotFound("Scene not found"))
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	_, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("nowhere-zz", strptr("Nothing here")))
	if err == nil {
		t.Fatal("expected an error for an unknown scene")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want a not-found error", err.Error())
	}
}

// The audit trail is what makes admin authoring reviewable: it must name the
// actor, the scene row the service actually wrote, and the field.
func TestUpdateSceneTaglineHandler_AuditsTheEntityEdit(t *testing.T) {
	h, rec := taglineHandler(t, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})

	if _, err := h.UpdateSceneTaglineHandler(ctx, taglineRequest("phoenix-az", strptr("Desert noise"))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.audited {
		t.Fatal("expected an entity-edit audit entry")
	}
	if rec.auditActor != 7 {
		t.Fatalf("audit actor = %d, want 7", rec.auditActor)
	}
	if rec.auditEntity != "scene" {
		t.Fatalf("audit entity type = %q, want scene", rec.auditEntity)
	}
	if rec.auditID != 42 {
		t.Fatalf("audit entity id = %d, want the scene id the service returned (42)", rec.auditID)
	}
	if field, _ := rec.auditMeta["field"].(string); field != "tagline" {
		t.Fatalf("audit metadata field = %v, want tagline", rec.auditMeta["field"])
	}
}
