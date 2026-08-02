package catalog

import (
	"context"
	"net"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

// The radio admin CRUD predates PSY-1675's guard, so image_url on a radio show
// was the one image_url the API accepted unclassified: any scheme, any host.
// These exercise the same corpus as the catalog write paths (ssrfImageURLs in
// image_url_ssrf_test.go) against both admin radio-show writers, and share that
// file's TestMain resolution table.
//
// Admin-gating is not a substitute for the check. The value is stored and later
// requested server-side by the share-card renderer, so a pasted or mistyped
// internal URL turns into an outbound request from our infrastructure regardless
// of who typed it.

// TestAdminCreateRadioShow_RejectsSSRFImageURL: POST /admin/radio-stations/{id}/shows.
func TestAdminCreateRadioShow_RejectsSSRFImageURL(t *testing.T) {
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			mock := &testhelpers.MockRadioService{
				CreateShowFn: func(uint, *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
					t.Fatal("the radio service must NOT be reached with an SSRF image_url")
					return nil, nil
				},
			}
			req := &AdminCreateRadioShowRequest{StationID: 1}
			req.Body.Name = "Morning Show"
			value := c.value
			req.Body.ImageURL = &value

			_, err := testRadioHandler(mock).AdminCreateRadioShowHandler(radioAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestAdminUpdateRadioShow_RejectsSSRFImageURL: PUT /admin/radio-shows/{id}.
func TestAdminUpdateRadioShow_RejectsSSRFImageURL(t *testing.T) {
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			mock := &testhelpers.MockRadioService{
				UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
					t.Fatal("the radio service must NOT be reached with an SSRF image_url")
					return nil, nil
				},
			}
			req := &AdminUpdateRadioShowRequest{ShowID: 1}
			value := c.value
			req.Body.ImageURL = &value

			_, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// cancelAwareResolver returns the caller's context error instead of answering,
// the way *net.Resolver does, and otherwise resolves the hostile fixture to the
// cloud metadata address. urlguard.MapResolver cannot stand in here because it
// ignores ctx entirely.
type cancelAwareResolver struct{}

func (cancelAwareResolver) LookupIPAddr(ctx context.Context, _ string) ([]net.IPAddr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
}

// TestAdminRadioShowWrites_GuardSurvivesCancelledContext reproduces the bypass
// an adversarial pass demonstrated end to end against these two writers: a
// cancelled request context made every DNS lookup fail, urlguard treats a failed
// lookup as a PASS, and the radio service takes no context, so the hostile value
// reached the write. Only the resolution step was lost, which is exactly the
// step that catches a NAME (as opposed to a literal) pointing inward.
//
// Fixed in urlguard.hostResolvesPublic rather than at these call sites, so the
// same hole cannot reopen on any other caller. Verified non-vacuous: dropping
// the context.WithoutCancel there makes both subtests fail.
func TestAdminRadioShowWrites_GuardSurvivesCancelledContext(t *testing.T) {
	// A NAME, not a literal. A literal is refused without any lookup and so
	// would pass this test even with the guard downgraded.
	const hostile = "https://rebind.example.test/x.jpg"

	// The package TestMain installs a MapResolver, which answers from a map and
	// never looks at ctx. Asserting cancellation through it would pass whether or
	// not the guard detaches, so swap in one that honours ctx the way
	// *net.Resolver does for the duration of this test.
	defer urlguard.Default.UseResolver(cancelAwareResolver{})()

	cancelledCtx := func() context.Context {
		ctx, cancel := context.WithCancel(radioAdminCtx())
		cancel()
		return ctx
	}

	t.Run("create", func(t *testing.T) {
		mock := &testhelpers.MockRadioService{
			CreateShowFn: func(uint, *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				t.Fatal("a cancelled context must not let an SSRF image_url reach the service")
				return nil, nil
			},
		}
		req := &AdminCreateRadioShowRequest{StationID: 1}
		req.Body.Name = "Morning Show"
		value := hostile
		req.Body.ImageURL = &value
		_, err := testRadioHandler(mock).AdminCreateRadioShowHandler(cancelledCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})

	t.Run("update", func(t *testing.T) {
		mock := &testhelpers.MockRadioService{
			UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				t.Fatal("a cancelled context must not let an SSRF image_url reach the service")
				return nil, nil
			},
		}
		req := &AdminUpdateRadioShowRequest{ShowID: 1}
		value := hostile
		req.Body.ImageURL = &value
		_, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(cancelledCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})
}

// TestAdminRadioShowWrites_RejectOverlongImageURL: radio_shows.image_url is
// VARCHAR(500) and neither body carries a maxLength tag, so without an explicit
// cap an over-long but otherwise valid URL cleared the guard and died at the
// driver as an opaque 500. It must be a 422 instead, on both writers.
func TestAdminRadioShowWrites_RejectOverlongImageURL(t *testing.T) {
	overlong := "https://example.com/" + strings.Repeat("a", 500) + ".jpg"

	t.Run("create", func(t *testing.T) {
		mock := &testhelpers.MockRadioService{
			CreateShowFn: func(uint, *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				t.Fatal("the radio service must NOT be reached with an over-long image_url")
				return nil, nil
			},
		}
		req := &AdminCreateRadioShowRequest{StationID: 1}
		req.Body.Name = "Morning Show"
		req.Body.ImageURL = &overlong
		_, err := testRadioHandler(mock).AdminCreateRadioShowHandler(radioAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})

	t.Run("update", func(t *testing.T) {
		mock := &testhelpers.MockRadioService{
			UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				t.Fatal("the radio service must NOT be reached with an over-long image_url")
				return nil, nil
			},
		}
		req := &AdminUpdateRadioShowRequest{ShowID: 1}
		req.Body.ImageURL = &overlong
		_, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})
}

// TestAdminRadioShowWrites_AcceptPublicImageURL confirms the guard did not close
// the ordinary path on either writer, and that a nil image_url (the common case,
// since the field is optional on both bodies) is untouched by it.
func TestAdminRadioShowWrites_AcceptPublicImageURL(t *testing.T) {
	const art = "https://example.com/show-art.jpg"

	t.Run("create with a public image", func(t *testing.T) {
		reached := false
		mock := &testhelpers.MockRadioService{
			CreateShowFn: func(_ uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				reached = true
				return &contracts.RadioShowDetailResponse{ID: 7, ImageURL: req.ImageURL}, nil
			},
		}
		req := &AdminCreateRadioShowRequest{StationID: 1}
		req.Body.Name = "Morning Show"
		value := art
		req.Body.ImageURL = &value

		if _, err := testRadioHandler(mock).AdminCreateRadioShowHandler(radioAdminCtx(), req); err != nil {
			t.Fatalf("a public image must still create, got: %v", err)
		}
		if !reached {
			t.Error("expected the create to reach the radio service")
		}
	})

	t.Run("update with no image at all", func(t *testing.T) {
		reached := false
		mock := &testhelpers.MockRadioService{
			UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
				reached = true
				return &contracts.RadioShowDetailResponse{ID: 7}, nil
			},
		}
		req := &AdminUpdateRadioShowRequest{ShowID: 7}
		name := "Renamed"
		req.Body.Name = &name

		if _, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req); err != nil {
			t.Fatalf("an update that does not touch image_url must still apply, got: %v", err)
		}
		if !reached {
			t.Error("expected the update to reach the radio service")
		}
	})
}
