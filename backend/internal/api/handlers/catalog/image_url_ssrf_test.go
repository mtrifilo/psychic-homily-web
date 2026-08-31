package catalog

import (
	"os"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

// TestMain pins the SSRF host guard (PSY-1675) to a fixed resolution table so
// the image_url write paths in this package are validated against known
// addresses rather than the machine's DNS. Package-level state: no test in this
// package may call t.Parallel while relying on it.
//
// example.test is reserved for documentation and never resolves in the real
// world, so the hostile fixture cannot accidentally reach a live host.
func TestMain(m *testing.M) {
	restore := urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	})
	code := m.Run()
	restore()
	os.Exit(code)
}

// ssrfImageURLs is the corpus PSY-1672's edge guard was hardened against, plus
// the hostname case only a resolver can see. Every entry is a value an
// email-verified submitter can put in image_url today.
var ssrfImageURLs = []struct{ name, value string }{
	{"cloud metadata literal", "https://169.254.169.254/latest/meta-data/"},
	{"cloud metadata over http", "http://169.254.169.254/latest/meta-data/"},
	{"ipv6-mapped metadata", "https://[::ffff:169.254.169.254]/x.jpg"},
	{"ipv6-mapped metadata, hex groups", "https://[::ffff:a9fe:a9fe]/x.jpg"},
	{"ipv4-compatible loopback", "https://[::127.0.0.1]/x.jpg"},
	{"decimal loopback", "https://2130706433/x.jpg"},
	{"hex loopback", "https://0x7f000001/x.jpg"},
	{"octal loopback", "https://0177.0.0.1/x.jpg"},
	{"short-form loopback", "https://127.1/x.jpg"},
	{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
	{"localhost", "https://localhost/x.jpg"},
	{"localhost with a trailing dot", "https://localhost./x.jpg"},
	{"gcp metadata by name", "https://metadata.google.internal/computeMetadata/v1/"},
	{"rfc1918", "https://10.0.0.5/x.jpg"},
	{"cgnat", "https://100.64.0.1/x.jpg"},
	{"oracle metadata", "https://192.0.0.192/x.jpg"},
	{"name resolving to cloud metadata", "https://rebind.example.test/x.jpg"},
}

// TestUpdateShowHandler_RejectsSSRFImageURL: PUT /shows/{id} is the path any
// email-verified submitter can reach for their own show, and image_url is the
// value the share-card renderer later fetches. The service must never see it.
func TestUpdateShowHandler_RejectsSSRFImageURL(t *testing.T) {
	for _, c := range ssrfImageURLs {
		t.Run(c.name, func(t *testing.T) {
			userID := uint(5)
			showMock := &testhelpers.MockShowService{
				GetShowFn: func(uint) (*contracts.ShowResponse, error) {
					return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID, Status: "pending"}, nil
				},
				UpdateShowWithRelationsFn: func(uint, *contracts.UpdateShowRequest, []contracts.CreateShowVenue, []contracts.CreateShowArtist, bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
					t.Fatal("the show service must NOT be reached with an SSRF image_url")
					return nil, nil, nil
				},
			}
			h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, testhelpers.AllShowsVisible())
			req := &UpdateShowRequest{ShowID: "1"}
			value := c.value
			req.Body.ImageURL = &value

			_, err := h.UpdateShowHandler(testhelpers.CtxWithUser(&authm.User{ID: 5}), req)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

// TestUpdateShowHandler_AcceptsPublicImageURL confirms the guard did not close
// the ordinary path: a flyer on a public host still updates.
func TestUpdateShowHandler_AcceptsPublicImageURL(t *testing.T) {
	userID := uint(5)
	reached := false
	showMock := &testhelpers.MockShowService{
		GetShowFn: func(uint) (*contracts.ShowResponse, error) {
			return &contracts.ShowResponse{ID: 1, SubmittedBy: &userID, Status: "pending"}, nil
		},
		UpdateShowWithRelationsFn: func(showID uint, _ *contracts.UpdateShowRequest, _ []contracts.CreateShowVenue, _ []contracts.CreateShowArtist, _ bool) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
			reached = true
			return &contracts.ShowResponse{ID: showID}, nil, nil
		},
	}
	h := NewShowHandler(showMock, nil, nil, nil, nil, nil, nil, testhelpers.AllShowsVisible())
	req := &UpdateShowRequest{ShowID: "1"}
	flyer := "https://example.com/flyer.jpg"
	req.Body.ImageURL = &flyer

	if _, err := h.UpdateShowHandler(testhelpers.CtxWithUser(&authm.User{ID: 5}), req); err != nil {
		t.Fatalf("a public flyer must still update, got: %v", err)
	}
	if !reached {
		t.Error("expected the update to reach the show service")
	}
}
