package admin

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

// TestMain pins the SSRF host guard (PSY-1675) to a fixed resolution table so
// the suggest-edit path's image_url check never depends on the machine's DNS.
// Package-level state: no test in this package may call t.Parallel while
// relying on it.
func TestMain(m *testing.M) {
	restore := urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	})
	code := m.Run()
	restore()
	os.Exit(code)
}

// TestSuggestEdit_RejectsSSRFImageURL: the suggest-edit path is the third
// surface an image_url can be written from, and ApprovePendingEdit applies a
// stored value blindly — so the classification has to happen here, at submit.
func TestSuggestEdit_RejectsSSRFImageURL(t *testing.T) {
	cases := []struct{ name, value string }{
		{"cloud metadata literal", "https://169.254.169.254/latest/meta-data/"},
		{"ipv6-mapped metadata", "https://[::ffff:169.254.169.254]/x.jpg"},
		{"decimal loopback", "https://2130706433/x.jpg"},
		{"octal loopback", "https://0177.0.0.1/x.jpg"},
		{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
		{"localhost with a trailing dot", "https://localhost./x.jpg"},
		{"rfc1918", "https://10.0.0.5/x.jpg"},
		{"name resolving to cloud metadata", "https://rebind.example.test/x.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A mock that fails the test rather than nil services that panic:
			// a nil-deref aborts the whole package binary, so one regression
			// here would destroy the signal from every other test in
			// internal/api/handlers/admin instead of failing this one case.
			h := NewPendingEditHandler(
				&testhelpers.MockPendingEditService{
					CreatePendingEditFn: func(*contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
						t.Fatal("the pending-edit service must NOT be reached with an SSRF image_url")
						return nil, nil
					},
				},
				nil,
			)
			req := &SuggestEntityEditRequest{EntityID: "1"}
			req.Body.Changes = []adminm.FieldChange{{Field: "image_url", OldValue: nil, NewValue: c.value}}
			req.Body.Summary = "swap the photo"
			_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestSuggestEdit_BoundsChangeFanOut: each URL-valued change can cost a DNS
// lookup, and this route has no rate limiter, so an unbounded or repetitive
// Changes array would be an outbound-request amplifier. Both bounds are
// enforced before any validation runs.
func TestSuggestEdit_BoundsChangeFanOut(t *testing.T) {
	handler := func(t *testing.T) *PendingEditHandler {
		t.Helper()
		return NewPendingEditHandler(
			&testhelpers.MockPendingEditService{
				CreatePendingEditFn: func(*contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
					t.Fatal("the service must NOT be reached for an over-large or repetitive change set")
					return nil, nil
				},
			},
			nil,
		)
	}

	t.Run("rejects more changes than the cap", func(t *testing.T) {
		changes := make([]adminm.FieldChange, maxPendingEditChanges+1)
		for i := range changes {
			changes[i] = adminm.FieldChange{Field: "image_url", NewValue: "https://example.com/x.jpg"}
		}
		req := &SuggestEntityEditRequest{EntityID: "1"}
		req.Body.Changes = changes
		req.Body.Summary = "flood"
		_, err := handler(t).SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})

	t.Run("rejects the same field listed twice", func(t *testing.T) {
		req := &SuggestEntityEditRequest{EntityID: "1"}
		req.Body.Changes = []adminm.FieldChange{
			{Field: "image_url", NewValue: "https://example.com/a.jpg"},
			{Field: "image_url", NewValue: "https://example.com/b.jpg"},
		}
		req.Body.Summary = "twice"
		_, err := handler(t).SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 422)
	})
}
