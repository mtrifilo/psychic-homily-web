package admin

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
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
		{"name that does not resolve", "https://nowhere.example.test/x.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// nil services: a rejection must happen before either is reached, so
			// anything that got past validation would nil-panic rather than pass.
			h := testPendingEditHandler()
			req := &SuggestEntityEditRequest{EntityID: "1"}
			req.Body.Changes = []adminm.FieldChange{{Field: "image_url", OldValue: nil, NewValue: c.value}}
			req.Body.Summary = "swap the photo"
			_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}
