package catalog

import (
	"context"
	"errors"
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

// hostileRadioImageURL is a NAME the package TestMain's resolver maps to cloud
// metadata, not a literal. A literal is refused without any lookup, so only a
// name proves that the resolution step actually ran (or, where a test asserts a
// value is let through, that skipping it was a real skip).
const hostileRadioImageURL = "https://rebind.example.test/x.jpg"

// TestAdminCreateRadioShow_RejectsSSRFImageURL: POST /admin/radio-stations/{id}/shows.
// Create has no stored value to compare against, so it validates every time and
// must never go looking for one.
func TestAdminCreateRadioShow_RejectsSSRFImageURL(t *testing.T) {
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			mock := &testhelpers.MockRadioService{
				GetShowFn: func(uint) (*contracts.RadioShowDetailResponse, error) {
					t.Fatal("create must not consult a stored value")
					return nil, nil
				},
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
// The row already holds an unrelated public image, so every corpus value is a
// CHANGE - which is the only case the update path validates.
func TestAdminUpdateRadioShow_RejectsSSRFImageURL(t *testing.T) {
	stored := "https://example.com/old-art.jpg"
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			mock := &testhelpers.MockRadioService{
				GetShowFn: func(uint) (*contracts.RadioShowDetailResponse, error) {
					return &contracts.RadioShowDetailResponse{ID: 1, ImageURL: &stored}, nil
				},
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
		value := hostileRadioImageURL
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
		value := hostileRadioImageURL
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

	// The row holds a different, short value, so the over-long one is a CHANGE.
	// Without the stub this would pass through the fail-closed branch instead and
	// never exercise the changed-value path the cap now lives behind.
	t.Run("update", func(t *testing.T) {
		stored := "https://example.com/old-art.jpg"
		mock := &testhelpers.MockRadioService{
			GetShowFn: func(uint) (*contracts.RadioShowDetailResponse, error) {
				return &contracts.RadioShowDetailResponse{ID: 1, ImageURL: &stored}, nil
			},
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

// The admin radio-show form re-sends image_url on every save, so a stored value
// that no longer clears the guard would 422 every later edit of that show if the
// guard ran unconditionally. The rest of this file pins the only-validate-when-
// changed rule from both sides.

// TestAdminUpdateRadioShow_UnchangedBadImageURLDoesNotBlockOtherEdits is the
// regression: a title-only edit that re-sends the stored (guard-failing) value
// byte for byte must reach the service.
func TestAdminUpdateRadioShow_UnchangedBadImageURLDoesNotBlockOtherEdits(t *testing.T) {
	reached := false
	stored := hostileRadioImageURL
	mock := &testhelpers.MockRadioService{
		GetShowFn: func(showID uint) (*contracts.RadioShowDetailResponse, error) {
			// The stored value must be read for the show being edited, not some
			// other row; a mismatched ID would compare against the wrong string.
			if showID != 7 {
				t.Errorf("stored value read for show %d, want 7", showID)
			}
			return &contracts.RadioShowDetailResponse{ID: 7, ImageURL: &stored}, nil
		},
		UpdateShowFn: func(_ uint, serviceReq *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			reached = true
			if serviceReq.Name == nil || *serviceReq.Name != "Renamed" {
				t.Error("expected the title edit to reach the service")
			}
			return &contracts.RadioShowDetailResponse{ID: 7}, nil
		},
	}
	req := &AdminUpdateRadioShowRequest{ShowID: 7}
	name := "Renamed"
	req.Body.Name = &name
	resent := hostileRadioImageURL
	req.Body.ImageURL = &resent

	if _, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req); err != nil {
		t.Fatalf("a stored guard-failing image_url must not block a title-only edit, got: %v", err)
	}
	if !reached {
		t.Error("expected the update to reach the radio service")
	}
}

// TestAdminUpdateRadioShow_UnchangedComparisonIsByteExact pins the comparison
// semantics. Each variant is a spelling that a resolver treats the same as the
// stored value, or that differs only cosmetically, and every one must count as a
// CHANGE and be validated. A laxer comparison is exactly how a hostile value
// would ride in labelled "unchanged".
func TestAdminUpdateRadioShow_UnchangedComparisonIsByteExact(t *testing.T) {
	variants := map[string]string{
		"uppercased host":      "https://REBIND.EXAMPLE.TEST/x.jpg",
		"trailing dot on host": "https://rebind.example.test./x.jpg",
		"percent-encoded path": "https://rebind.example.test/%78.jpg",
		"leading whitespace":   " https://rebind.example.test/x.jpg",
	}
	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			stored := hostileRadioImageURL
			mock := &testhelpers.MockRadioService{
				GetShowFn: func(uint) (*contracts.RadioShowDetailResponse, error) {
					return &contracts.RadioShowDetailResponse{ID: 7, ImageURL: &stored}, nil
				},
				UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
					t.Fatal("a value that is not byte-identical to the stored one must be validated")
					return nil, nil
				},
			}
			req := &AdminUpdateRadioShowRequest{ShowID: 7}
			value := variant
			req.Body.ImageURL = &value

			_, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestAdminUpdateRadioShow_UnreadableStoredValueStillValidates pins the
// fail-closed half: when the stored value cannot be read, the skip must not
// happen. Otherwise a caller who can make that read fail gets an unguarded
// write, which is a worse hole than the one this rule closes.
func TestAdminUpdateRadioShow_UnreadableStoredValueStillValidates(t *testing.T) {
	for name, getShow := range map[string]func(uint) (*contracts.RadioShowDetailResponse, error){
		"read fails":   func(uint) (*contracts.RadioShowDetailResponse, error) { return nil, errors.New("db down") },
		"no such show": func(uint) (*contracts.RadioShowDetailResponse, error) { return nil, nil },
		"no stored value": func(uint) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 7}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			mock := &testhelpers.MockRadioService{
				GetShowFn: getShow,
				UpdateShowFn: func(uint, *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
					t.Fatal("an unreadable stored value must not skip the guard")
					return nil, nil
				},
			}
			req := &AdminUpdateRadioShowRequest{ShowID: 7}
			value := hostileRadioImageURL
			req.Body.ImageURL = &value

			_, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestAdminUpdateRadioShow_ClearingStoredBadImageURLReachesTheService: an empty
// string is the spelling that actually clears the column, and the guard must let
// it through - otherwise a row whose stored value the guard now refuses could
// never be emptied.
//
// The admin form does NOT send this spelling today: it sends JSON null for an
// emptied field, and the service writes image_url only when the pointer is
// non-nil (services/catalog/radio.go UpdateShow), so emptying the field in the
// UI is a silent no-op that leaves the bad value in place. That is a pre-existing
// bug in the form, out of scope here; the in-product way out on this path is to
// overwrite with a good URL, which validates and applies.
func TestAdminUpdateRadioShow_ClearingStoredBadImageURLReachesTheService(t *testing.T) {
	written := false
	stored := hostileRadioImageURL
	mock := &testhelpers.MockRadioService{
		GetShowFn: func(uint) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 7, ImageURL: &stored}, nil
		},
		UpdateShowFn: func(_ uint, serviceReq *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			written = true
			if serviceReq.ImageURL == nil || *serviceReq.ImageURL != "" {
				t.Errorf("the empty value must reach the service verbatim, got %v", serviceReq.ImageURL)
			}
			return &contracts.RadioShowDetailResponse{ID: 7}, nil
		},
	}
	req := &AdminUpdateRadioShowRequest{ShowID: 7}
	empty := ""
	req.Body.ImageURL = &empty

	if _, err := testRadioHandler(mock).AdminUpdateRadioShowHandler(radioAdminCtx(), req); err != nil {
		t.Fatalf("clearing a stored guard-failing image_url must apply, got: %v", err)
	}
	if !written {
		t.Error("expected the clear to reach the radio service")
	}
}
