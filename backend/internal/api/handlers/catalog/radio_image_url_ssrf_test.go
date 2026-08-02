package catalog

import (
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
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
